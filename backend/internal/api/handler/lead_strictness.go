package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// LeadStrictnessHandler serves the workspace's lead line: how sure a post
// must be to show as a lead.
type LeadStrictnessHandler struct{ q *database.Queries }

func NewLeadStrictnessHandler(q *database.Queries) *LeadStrictnessHandler {
	return &LeadStrictnessHandler{q: q}
}

type leadStrictnessResponse struct {
	Strictness database.LeadStrictness `json:"strictness"`
	MinScore   float64                 `json:"min_score"`
}

func toLeadStrictnessResponse(s database.LeadStrictness) leadStrictnessResponse {
	return leadStrictnessResponse{Strictness: s, MinScore: s.MinScore()}
}

// Get — GET /settings/lead-strictness
func (h *LeadStrictnessHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.q.GetLeadStrictness(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read lead strictness")
		return
	}
	writeJSON(w, http.StatusOK, toLeadStrictnessResponse(s))
}

// Put — PUT /settings/lead-strictness {"strictness": "strict"|"balanced"|"loose"}
func (h *LeadStrictnessHandler) Put(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Strictness string `json:"strictness"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	s, err := database.ParseLeadStrictness(body.Strictness)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	if err := h.q.SetLeadStrictness(ctx, middleware.WorkspaceID(ctx), s); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save lead strictness")
		return
	}
	writeJSON(w, http.StatusOK, toLeadStrictnessResponse(s))
}
