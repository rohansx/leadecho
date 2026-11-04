package handler

// enrich (P2 #16): LinkedIn engagers -> leads.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/engagers"
)

type engagerPuller interface {
	Enabled() bool
	Pull(ctx context.Context, ws string, req engagers.Request) ([]engagers.Row, error)
}

type leadGetter interface {
	GetLead(ctx context.Context, arg database.GetLeadParams) (database.Lead, error)
}

type crmEmitter interface {
	Emit(wsID string, ev crmsync.Event)
}

type EngagersHandler struct {
	puller engagerPuller
	leads  leadGetter
	crm    crmEmitter
}

func NewEngagersHandler(p *engagers.Importer, q *database.Queries, crm *crmsync.Dispatcher) *EngagersHandler {
	h := &EngagersHandler{puller: p, leads: q}
	if crm != nil { // avoid a typed-nil interface
		h.crm = crm
	}
	return h
}

const (
	maxPushLeads = 200
	// Under the API server's 30s WriteTimeout.
	engagersTimeout = 25 * time.Second
)

// Pull handles POST /api/v1/linkedin/engagers {post_url, kinds, enrich}.
func (h *EngagersHandler) Pull(w http.ResponseWriter, r *http.Request) {
	if h.puller == nil || !h.puller.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "LinkedIn engagers need treg (TREG_TOKEN) on this server")
		return
	}
	var body struct {
		PostURL string   `json:"post_url"`
		Kinds   []string `json:"kinds"`
		Enrich  bool     `json:"enrich"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ref, err := engagers.ParsePostRef(body.PostURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kinds, err := engagers.ValidKinds(body.Kinds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), engagersTimeout)
	defer cancel()
	rows, err := h.puller.Pull(ctx, middleware.WorkspaceID(ctx), engagers.Request{PostURL: body.PostURL, Ref: ref, Kinds: kinds, Enrich: body.Enrich})
	switch {
	case errors.Is(err, engagers.ErrNoData):
		writeError(w, http.StatusBadGateway, "could not load engagers for this post, try again later")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to import engagers")
		return
	}
	if rows == nil {
		rows = []engagers.Row{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "total": len(rows)})
}

// Push handles POST /api/v1/linkedin/engagers/push {lead_ids}: queue a CRM
// sync for each lead of this workspace (others are ignored).
func (h *EngagersHandler) Push(w http.ResponseWriter, r *http.Request) {
	if h.crm == nil {
		writeError(w, http.StatusServiceUnavailable, "CRM sync is not configured on this server")
		return
	}
	var body struct {
		LeadIDs []string `json:"lead_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(body.LeadIDs) == 0 || len(body.LeadIDs) > maxPushLeads {
		writeError(w, http.StatusBadRequest, "lead_ids must have 1-200 ids")
		return
	}
	for _, id := range body.LeadIDs {
		if !parseUUID(id).Valid {
			writeError(w, http.StatusBadRequest, "invalid lead id")
			return
		}
	}
	ctx := r.Context()
	ws := middleware.WorkspaceID(ctx)
	queued := 0
	for _, id := range body.LeadIDs {
		if _, err := h.leads.GetLead(ctx, database.GetLeadParams{ID: id, WorkspaceID: ws}); err != nil {
			continue
		}
		// lead.created upserts: sinks create the contact once, then update it.
		h.crm.Emit(ws, crmsync.Event{Type: crmsync.EventLeadCreated, LeadID: id})
		queued++
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": queued})
}
