package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type ProposalHandler struct {
	q *database.Queries
}

func NewProposalHandler(q *database.Queries) *ProposalHandler {
	return &ProposalHandler{q: q}
}

func (h *ProposalHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	limit := int32(30)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}

	rows, err := h.q.ListHumanProposals(r.Context(), database.ListHumanProposalsParams{
		WorkspaceID: wsID,
		Status:      status,
		Lim:         limit,
		Off:         0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list proposals")
		return
	}
	if rows == nil {
		rows = []database.HumanProposal{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *ProposalHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Status != "pending" && body.Status != "accepted" && body.Status != "dismissed" {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	row, err := h.q.UpdateHumanProposalStatus(r.Context(), database.UpdateHumanProposalStatusParams{
		ID:          id,
		WorkspaceID: wsID,
		Status:      body.Status,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "proposal not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *ProposalHandler) Counts(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	counts, err := h.q.CountHumanProposalsByStatus(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count proposals")
		return
	}
	writeJSON(w, http.StatusOK, counts)
}
