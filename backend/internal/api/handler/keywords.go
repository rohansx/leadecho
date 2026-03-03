package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type KeywordHandler struct {
	q *database.Queries
}

func NewKeywordHandler(q *database.Queries) *KeywordHandler {
	return &KeywordHandler{q: q}
}

type KeywordResponse struct {
	ID            string   `json:"id"`
	WorkspaceID   string   `json:"workspace_id"`
	Term          string   `json:"term"`
	Platforms     []string `json:"platforms"`
	IsActive      bool     `json:"is_active"`
	MatchType     string   `json:"match_type"`
	NegativeTerms []string `json:"negative_terms"`
	Subreddits    []string `json:"subreddits"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func toKeywordResponse(id, wsID, term string, platforms []string, isActive bool, matchType string, negTerms, subreddits []string, createdAt, updatedAt time.Time) KeywordResponse {
	return KeywordResponse{
		ID:            id,
		WorkspaceID:   wsID,
		Term:          term,
		Platforms:     platforms,
		IsActive:      isActive,
		MatchType:     matchType,
		NegativeTerms: negTerms,
		Subreddits:    subreddits,
		CreatedAt:     createdAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     updatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *KeywordHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	keywords, err := h.q.ListKeywords(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list keywords")
		return
	}
	resp := make([]KeywordResponse, len(keywords))
	for i, k := range keywords {
		resp[i] = toKeywordResponse(k.ID, k.WorkspaceID, k.Term, k.Platforms, k.IsActive, k.MatchType, k.NegativeTerms, k.Subreddits, k.CreatedAt, k.UpdatedAt)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *KeywordHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	k, err := h.q.GetKeyword(r.Context(), database.GetKeywordParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "keyword not found")
		return
	}
	writeJSON(w, http.StatusOK, toKeywordResponse(k.ID, k.WorkspaceID, k.Term, k.Platforms, k.IsActive, k.MatchType, k.NegativeTerms, k.Subreddits, k.CreatedAt, k.UpdatedAt))
}

func (h *KeywordHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Term          string   `json:"term"`
		Platforms     []string `json:"platforms"`
		MatchType     string   `json:"match_type"`
		NegativeTerms []string `json:"negative_terms"`
		Subreddits    []string `json:"subreddits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Term == "" {
		writeError(w, http.StatusBadRequest, "term is required")
		return
	}
	if body.MatchType == "" {
		body.MatchType = "contains"
	}
	if body.Platforms == nil {
		body.Platforms = []string{"hackernews", "reddit", "twitter", "linkedin"}
	}
	if body.NegativeTerms == nil {
		body.NegativeTerms = []string{}
	}
	if body.Subreddits == nil {
		body.Subreddits = []string{}
	}

	k, err := h.q.CreateKeyword(r.Context(), database.CreateKeywordParams{
		WorkspaceID:   wsID,
		Term:          body.Term,
		Platforms:     body.Platforms,
		IsActive:      true,
		MatchType:     body.MatchType,
		NegativeTerms: body.NegativeTerms,
		Subreddits:    body.Subreddits,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create keyword")
		return
	}
	writeJSON(w, http.StatusCreated, toKeywordResponse(k.ID, k.WorkspaceID, k.Term, k.Platforms, k.IsActive, k.MatchType, k.NegativeTerms, k.Subreddits, k.CreatedAt, k.UpdatedAt))
}

func (h *KeywordHandler) Update(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Term          string   `json:"term"`
		Platforms     []string `json:"platforms"`
		IsActive      *bool    `json:"is_active"`
		MatchType     string   `json:"match_type"`
		NegativeTerms []string `json:"negative_terms"`
		Subreddits    []string `json:"subreddits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	existing, err := h.q.GetKeyword(r.Context(), database.GetKeywordParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "keyword not found")
		return
	}

	term := existing.Term
	if body.Term != "" {
		term = body.Term
	}
	matchType := existing.MatchType
	if body.MatchType != "" {
		matchType = body.MatchType
	}
	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	platforms := existing.Platforms
	if body.Platforms != nil {
		platforms = body.Platforms
	}

	negativeTerms := existing.NegativeTerms
	if body.NegativeTerms != nil {
		negativeTerms = body.NegativeTerms
	}

	subreddits := existing.Subreddits
	if body.Subreddits != nil {
		subreddits = body.Subreddits
	}

	k, err := h.q.UpdateKeyword(r.Context(), database.UpdateKeywordParams{
		ID:            id,
		WorkspaceID:   wsID,
		Term:          term,
		Platforms:     platforms,
		IsActive:      isActive,
		MatchType:     matchType,
		NegativeTerms: negativeTerms,
		Subreddits:    subreddits,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update keyword")
		return
	}
	writeJSON(w, http.StatusOK, toKeywordResponse(k.ID, k.WorkspaceID, k.Term, k.Platforms, k.IsActive, k.MatchType, k.NegativeTerms, k.Subreddits, k.CreatedAt, k.UpdatedAt))
}

func (h *KeywordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.q.DeleteKeyword(r.Context(), database.DeleteKeywordParams{ID: id, WorkspaceID: wsID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete keyword")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
