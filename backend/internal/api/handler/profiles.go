package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/llm"
)

// storedPhrases returns the pain-point phrases actually persisted for a profile,
// so API responses never report phrases that weren't saved (e.g. when no
// embedder is configured or embedding failed).
func (h *ProfileHandler) storedPhrases(ctx context.Context, profileID string) []string {
	embeddings, _ := h.q.ListPainPointEmbeddings(ctx, profileID)
	phrases := make([]string, len(embeddings))
	for j, e := range embeddings {
		phrases[j] = e.Phrase
	}
	return phrases
}

type ProfileHandler struct {
	q         *database.Queries
	llmRouter *llm.Router
}

func NewProfileHandler(q *database.Queries, llmRouter *llm.Router) *ProfileHandler {
	return &ProfileHandler{q: q, llmRouter: llmRouter}
}

type ProfileResponse struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspace_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	PainPoints  []string `json:"pain_points"`
	IsActive    bool     `json:"is_active"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func toProfileResponse(p database.MonitoringProfile, phrases []string) ProfileResponse {
	if phrases == nil {
		phrases = []string{}
	}
	return ProfileResponse{
		ID:          p.ID,
		WorkspaceID: p.WorkspaceID,
		Name:        p.Name,
		Description: p.Description,
		PainPoints:  phrases,
		IsActive:    p.IsActive,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	profiles, err := h.q.ListMonitoringProfiles(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}

	resp := make([]ProfileResponse, len(profiles))
	for i, p := range profiles {
		embeddings, _ := h.q.ListPainPointEmbeddings(r.Context(), p.ID)
		phrases := make([]string, len(embeddings))
		for j, e := range embeddings {
			phrases[j] = e.Phrase
		}
		resp[i] = toProfileResponse(p, phrases)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	p, err := h.q.GetMonitoringProfile(r.Context(), database.GetMonitoringProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	embeddings, _ := h.q.ListPainPointEmbeddings(r.Context(), p.ID)
	phrases := make([]string, len(embeddings))
	for j, e := range embeddings {
		phrases[j] = e.Phrase
	}
	writeJSON(w, http.StatusOK, toProfileResponse(p, phrases))
}

func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		PainPoints  []string `json:"pain_points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.PainPoints == nil {
		body.PainPoints = []string{}
	}

	profile, err := h.q.CreateMonitoringProfile(r.Context(), database.CreateMonitoringProfileParams{
		WorkspaceID: wsID,
		Name:        body.Name,
		Description: body.Description,
		IsActive:    true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}

	// Embed and store pain-point phrases (best effort — requires an embedder).
	if len(body.PainPoints) > 0 && h.llmRouter != nil {
		_ = h.embedAndStorePhrases(r.Context(), profile.ID, wsID, body.PainPoints)
	}

	// Respond with the phrases that were actually persisted, not the optimistic
	// request body (which would lie when no embedder is configured).
	writeJSON(w, http.StatusCreated, toProfileResponse(profile, h.storedPhrases(r.Context(), profile.ID)))
}

func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		PainPoints  []string `json:"pain_points"`
		IsActive    *bool    `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	existing, err := h.q.GetMonitoringProfile(r.Context(), database.GetMonitoringProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	name := existing.Name
	if strings.TrimSpace(body.Name) != "" {
		name = strings.TrimSpace(body.Name)
	}
	description := existing.Description
	if body.Description != "" {
		description = body.Description
	}
	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	profile, err := h.q.UpdateMonitoringProfile(r.Context(), database.UpdateMonitoringProfileParams{
		ID:          id,
		WorkspaceID: wsID,
		Name:        name,
		Description: description,
		IsActive:    isActive,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	// Re-embed phrases if provided. Embed FIRST and only replace the existing
	// embeddings once the (failable) embed succeeds, so a transient embedding
	// failure never wipes the profile's pain points (data-loss guard).
	if body.PainPoints != nil && h.llmRouter != nil {
		if len(body.PainPoints) > 0 {
			vectors, err := h.llmRouter.EmbedTexts(r.Context(), wsID, llm.TaskEmbedProfiles, body.PainPoints)
			if err != nil {
				writeError(w, http.StatusBadGateway, "failed to embed pain points; existing phrases preserved")
				return
			}
			h.q.DeletePainPointEmbeddingsByProfile(r.Context(), id)
			for i, phrase := range body.PainPoints {
				if i >= len(vectors) {
					break
				}
				h.q.CreatePainPointEmbedding(r.Context(), database.CreatePainPointEmbeddingParams{
					ProfileID:   id,
					WorkspaceID: wsID,
					Phrase:      phrase,
					Embedding:   &vectors[i],
				})
			}
		} else {
			// Explicit empty list → clear stored phrases.
			h.q.DeletePainPointEmbeddingsByProfile(r.Context(), id)
		}
	}

	writeJSON(w, http.StatusOK, toProfileResponse(profile, h.storedPhrases(r.Context(), id)))
}

func (h *ProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}
	// Confirm ownership so non-existent / cross-workspace ids return 404 rather
	// than a false "deleted".
	if _, err := h.q.GetMonitoringProfile(r.Context(), database.GetMonitoringProfileParams{ID: id, WorkspaceID: wsID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}
	if err := h.q.DeleteMonitoringProfile(r.Context(), database.DeleteMonitoringProfileParams{ID: id, WorkspaceID: wsID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *ProfileHandler) embedAndStorePhrases(ctx context.Context, profileID, wsID string, phrases []string) error {
	vectors, err := h.llmRouter.EmbedTexts(ctx, wsID, llm.TaskEmbedProfiles, phrases)
	if err != nil {
		return err
	}

	for i, phrase := range phrases {
		if i >= len(vectors) {
			break
		}
		h.q.CreatePainPointEmbedding(ctx, database.CreatePainPointEmbeddingParams{
			ProfileID:   profileID,
			WorkspaceID: wsID,
			Phrase:      phrase,
			Embedding:   &vectors[i],
		})
	}
	return nil
}
