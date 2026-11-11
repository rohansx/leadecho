package handler

// demand: what the market is asking for, grouped into themes, with the
// people behind each one and a follow-up once a theme ships.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/demand"
)

type DemandHandler struct {
	svc    *demand.Service
	logger zerolog.Logger
}

func NewDemandHandler(svc *demand.Service, logger zerolog.Logger) *DemandHandler {
	return &DemandHandler{svc: svc, logger: logger}
}

type demandResponse struct {
	Themes  []demand.ThemeView `json:"themes"`
	LastRun *time.Time         `json:"last_run"`
	// Waiting is how many asks are not in a theme yet; grouping runs daily,
	// or on request once at least MinNewAsks are waiting.
	Waiting        int `json:"waiting"`
	MinNewAsks     int `json:"min_new_asks"`
	MinThemePeople int `json:"min_theme_people"`
	FollowUpCost   int `json:"follow_up_cost"`
	MaxFollowUps   int `json:"max_follow_ups"`
}

// Get (GET /demand) returns the workspace's themes.
func (h *DemandHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceID(r.Context())
	state, err := h.svc.Store.LoadState(r.Context(), ws)
	if err != nil {
		h.logger.Error().Err(err).Msg("demand: load")
		writeError(w, http.StatusInternalServerError, "failed to load demand themes")
		return
	}
	resp := demandResponse{
		Themes: state.View(time.Now()), MinNewAsks: demand.MinNewAsks, MinThemePeople: demand.MinThemePeople,
		FollowUpCost: demand.FollowUpCost, MaxFollowUps: demand.MaxFollowUps,
	}
	if !state.LastRun.IsZero() {
		resp.LastRun = &state.LastRun
	}
	if recent, err := h.svc.Store.RecentAsks(r.Context(), ws); err == nil {
		resp.Waiting = len(state.Unassigned(recent))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Refresh (POST /demand/refresh) groups the waiting asks now.
func (h *DemandHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceID(r.Context())
	placed, err := h.svc.Refresh(r.Context(), ws, time.Now().UTC())
	if err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", ws).Msg("demand: refresh")
		writeError(w, http.StatusBadGateway, "could not group the asks right now; try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"placed": placed})
}

// Ship (POST /demand/themes/{id}/ship) marks a theme as shipped and,
// optionally, drafts a follow-up for the people who asked.
func (h *DemandHandler) Ship(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceID(r.Context())
	var body struct {
		Note   string `json:"note"`
		Drafts bool   `json:"drafts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Note = strings.TrimSpace(body.Note)
	if body.Note == "" || utf8.RuneCountInString(body.Note) > 600 {
		writeError(w, http.StatusBadRequest, "say what you shipped in 1-600 characters")
		return
	}
	theme, drafted, err := h.svc.Ship(r.Context(), ws, chi.URLParam(r, "id"), body.Note, body.Drafts, time.Now().UTC())
	if errors.Is(err, demand.ErrNoTheme) {
		writeError(w, http.StatusNotFound, "theme not found")
		return
	}
	if errors.Is(err, demand.ErrShipped) {
		writeError(w, http.StatusConflict, "this theme is already marked as shipped")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Str("workspace_id", ws).Msg("demand: ship")
		writeError(w, http.StatusInternalServerError, "failed to mark the theme as shipped")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme_id": theme.ID, "status": theme.Status, "drafted": drafted})
}
