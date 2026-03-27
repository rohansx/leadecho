package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
)

type ProfileHandler struct {
	q        *database.Queries
	embedder *embedding.Client
}

func NewProfileHandler(q *database.Queries, embedder *embedding.Client) *ProfileHandler {
	return &ProfileHandler{q: q, embedder: embedder}
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

	// Embed and store pain-point phrases
	if len(body.PainPoints) > 0 && h.embedder != nil {
		if err := h.embedAndStorePhrases(r.Context(), profile.ID, wsID, body.PainPoints); err != nil {
			// Profile created but embedding failed — log and continue
			writeJSON(w, http.StatusCreated, toProfileResponse(profile, body.PainPoints))
			return
		}
	}

	writeJSON(w, http.StatusCreated, toProfileResponse(profile, body.PainPoints))
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
	if body.Name != "" {
		name = body.Name
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

	// Re-embed phrases if provided
	if body.PainPoints != nil && h.embedder != nil {
		h.q.DeletePainPointEmbeddingsByProfile(r.Context(), id)
		if len(body.PainPoints) > 0 {
			h.embedAndStorePhrases(r.Context(), id, wsID, body.PainPoints)
		}
	}

	// Fetch current phrases
	embeddings, _ := h.q.ListPainPointEmbeddings(r.Context(), id)
	phrases := make([]string, len(embeddings))
	for j, e := range embeddings {
		phrases[j] = e.Phrase
	}

	writeJSON(w, http.StatusOK, toProfileResponse(profile, phrases))
}

func (h *ProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.q.DeleteMonitoringProfile(r.Context(), database.DeleteMonitoringProfileParams{ID: id, WorkspaceID: wsID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *ProfileHandler) embedAndStorePhrases(ctx context.Context, profileID, wsID string, phrases []string) error {
	vectors, err := h.embedder.EmbedTexts(ctx, phrases)
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
