package handler

// sources: mute lists CRUD (/api/v1/mutes).

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/mutes"
)

type MuteHandler struct{ store *mutes.Store }

func NewMuteHandler(store *mutes.Store) *MuteHandler { return &MuteHandler{store: store} }

type MuteResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *MuteHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.List(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list mutes")
		return
	}
	out := make([]MuteResponse, len(rows))
	for i, m := range rows {
		out[i] = MuteResponse{ID: m.ID, Kind: m.Kind, Value: m.Value, CreatedAt: m.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *MuteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, err := mutes.Normalize(req.Kind, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	m, err := h.store.AddBy(ctx, middleware.WorkspaceID(ctx), middleware.UserID(ctx), req.Kind, req.Value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create mute")
		return
	}
	writeJSON(w, http.StatusCreated, MuteResponse{ID: m.ID, Kind: m.Kind, Value: m.Value, CreatedAt: m.CreatedAt})
}

func (h *MuteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid mute id")
		return
	}
	ok, err := h.store.Delete(r.Context(), middleware.WorkspaceID(r.Context()), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete mute")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "mute not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
