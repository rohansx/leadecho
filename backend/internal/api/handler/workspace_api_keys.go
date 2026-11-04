package handler

// mcp: workspace API keys (MCP server + public REST). Not the BYOK
// /settings/api-keys, which stores the workspace's LLM provider keys.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

const maxAPIKeyName = 100

type workspaceAPIKeyStore interface {
	CreateWorkspaceAPIKey(ctx context.Context, arg database.CreateWorkspaceAPIKeyParams) (database.WorkspaceApiKey, error)
	ListWorkspaceAPIKeys(ctx context.Context, workspaceID string) ([]database.WorkspaceApiKey, error)
	RevokeWorkspaceAPIKey(ctx context.Context, arg database.RevokeWorkspaceAPIKeyParams) (database.WorkspaceApiKey, error)
}

type WorkspaceAPIKeyHandler struct{ q workspaceAPIKeyStore }

func NewWorkspaceAPIKeyHandler(q workspaceAPIKeyStore) *WorkspaceAPIKeyHandler {
	return &WorkspaceAPIKeyHandler{q: q}
}

type workspaceAPIKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	Revoked    bool       `json:"revoked"`
}

func toWorkspaceAPIKeyResponse(k database.WorkspaceApiKey) workspaceAPIKeyResponse {
	r := workspaceAPIKeyResponse{ID: k.ID, Name: k.Name, Prefix: k.Prefix, Scopes: k.Scopes, CreatedAt: k.CreatedAt, Revoked: k.RevokedAt.Valid}
	if k.LastUsedAt.Valid {
		r.LastUsedAt = &k.LastUsedAt.Time
	}
	if k.RevokedAt.Valid {
		r.RevokedAt = &k.RevokedAt.Time
	}
	return r
}

func (h *WorkspaceAPIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	keys, err := h.q.ListWorkspaceAPIKeys(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}
	out := make([]workspaceAPIKeyResponse, len(keys))
	for i, k := range keys {
		out[i] = toWorkspaceAPIKeyResponse(k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Create returns the plaintext key exactly once; only its hash is stored.
func (h *WorkspaceAPIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || utf8.RuneCountInString(body.Name) > maxAPIKeyName {
		writeError(w, http.StatusBadRequest, "name is required (max 100 characters)")
		return
	}
	scopes, err := apikey.NormalizeScopes(body.Scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	key, err := apikey.Generate()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	row, err := h.q.CreateWorkspaceAPIKey(r.Context(), database.CreateWorkspaceAPIKeyParams{
		WorkspaceID: middleware.WorkspaceID(r.Context()),
		Name:        body.Name,
		Prefix:      key.Display,
		KeyHash:     key.Hash,
		Scopes:      scopes,
		CreatedBy:   parseUUID(middleware.UserID(r.Context())),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": toWorkspaceAPIKeyResponse(row), "secret": key.Plaintext})
}

func (h *WorkspaceAPIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	row, err := h.q.RevokeWorkspaceAPIKey(r.Context(), database.RevokeWorkspaceAPIKeyParams{ID: id, WorkspaceID: middleware.WorkspaceID(r.Context())})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke API key")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceAPIKeyResponse(row))
}
