package handler

// competitor watch (/api/v1/competitors): the competitors LeadEcho found for
// a workspace, where each one publishes, and the moves read out of that.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/competitors"
	"leadecho/internal/database"
	"leadecho/internal/onboarding"
)

type CompetitorsHandler struct {
	svc    *competitors.Service
	logger zerolog.Logger
}

func NewCompetitorsHandler(svc *competitors.Service, logger zerolog.Logger) *CompetitorsHandler {
	return &CompetitorsHandler{svc: svc, logger: logger}
}

func (h *CompetitorsHandler) fail(w http.ResponseWriter, err error, what string) {
	h.logger.Error().Err(err).Msg("competitors: " + what)
	writeError(w, http.StatusInternalServerError, "failed to "+what)
}

// List (GET /competitors) returns the page view.
func (h *CompetitorsHandler) List(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Load(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		h.fail(w, err, "load competitors")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Setup (POST /competitors/setup) runs the automatic setup again: it looks
// for the competitors named in the brand profile that are not watched yet.
func (h *CompetitorsHandler) Setup(w http.ResponseWriter, r *http.Request) {
	created, unresolved, err := h.svc.Setup(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		h.fail(w, err, "find competitors")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "unresolved": unresolved})
}

// Create (POST /competitors) starts watching a competitor. With only a name
// (a one-click suggestion) the website is looked up; a name that cannot be
// matched to a site with confidence is refused, never guessed.
func (h *CompetitorsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceID(r.Context())
	var body struct {
		Name    string `json:"name"`
		Website string `json:"website"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 80 {
		writeError(w, http.StatusBadRequest, "name is required (80 characters at most)")
		return
	}
	site := ""
	if strings.TrimSpace(body.Website) != "" {
		u, err := onboarding.NormalizeProductURL(body.Website)
		if err != nil {
			writeError(w, http.StatusBadRequest, "website must be an http(s) URL")
			return
		}
		site = u
	} else {
		if site = h.svc.Resolve(r.Context(), name, ""); site == "" {
			writeError(w, http.StatusUnprocessableEntity, "couldn't find their website: add it to start watching")
			return
		}
	}
	c, err := h.svc.Add(r.Context(), ws, name, site)
	if errors.Is(err, competitors.ErrLimit) {
		writeError(w, http.StatusConflict, "your plan's competitor limit is reached: remove one or upgrade")
		return
	}
	if err != nil {
		h.fail(w, err, "add competitor")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": c})
}

// Update (PATCH /competitors/{id}) pauses or resumes a competitor, or fixes
// its website. A new website replaces the sources with what that site links to.
func (h *CompetitorsHandler) Update(w http.ResponseWriter, r *http.Request) {
	ws, id := middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	var body struct {
		Website *string `json:"website"`
		Status  *string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cur, ok := h.load(w, r, ws, id)
	if !ok {
		return
	}
	next := database.UpdateCompetitorParams{ID: id, WorkspaceID: ws, Website: cur.Website, Status: cur.Status}
	if body.Status != nil {
		if *body.Status != "active" && *body.Status != "paused" {
			writeError(w, http.StatusBadRequest, "status must be active or paused")
			return
		}
		next.Status = *body.Status
	}
	if body.Website != nil {
		u, err := onboarding.NormalizeProductURL(*body.Website)
		if err != nil {
			writeError(w, http.StatusBadRequest, "website must be an http(s) URL")
			return
		}
		next.Website = u
	}
	c, err := h.svc.Q.UpdateCompetitor(r.Context(), next)
	if err != nil {
		h.fail(w, err, "update competitor")
		return
	}
	if c.Website != cur.Website {
		if err := h.svc.Q.DeleteCompetitorSources(r.Context(), database.DeleteCompetitorSourcesParams{CompetitorID: id, WorkspaceID: ws}); err != nil {
			h.fail(w, err, "replace sources")
			return
		}
		if err := h.svc.Rediscover(r.Context(), c); err != nil {
			h.fail(w, err, "read the new website")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": c})
}

// load fetches a competitor of this workspace or writes 404.
func (h *CompetitorsHandler) load(w http.ResponseWriter, r *http.Request, ws, id string) (database.Competitor, bool) {
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "competitor not found")
		return database.Competitor{}, false
	}
	c, err := h.svc.Q.GetCompetitor(r.Context(), database.GetCompetitorParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "competitor not found")
		return database.Competitor{}, false
	}
	if err != nil {
		h.fail(w, err, "load competitor")
		return database.Competitor{}, false
	}
	return c, true
}

func (h *CompetitorsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ws, id := middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	if _, ok := h.load(w, r, ws, id); !ok {
		return
	}
	if _, err := h.svc.Q.DeleteCompetitor(r.Context(), database.DeleteCompetitorParams{ID: id, WorkspaceID: ws}); err != nil {
		h.fail(w, err, "remove competitor")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AddSource (POST /competitors/{id}/sources) adds a place to watch by hand.
func (h *CompetitorsHandler) AddSource(w http.ResponseWriter, r *http.Request) {
	ws, id := middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	var body struct {
		Kind string `json:"kind"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	u, err := competitors.NormalizeSource(body.Kind, body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := h.load(w, r, ws, id); !ok {
		return
	}
	n, err := h.svc.Q.CountCompetitorSources(r.Context(), id)
	if err != nil {
		h.fail(w, err, "count sources")
		return
	}
	if int(n) >= competitors.MaxSources {
		writeError(w, http.StatusConflict, "a competitor can have at most 10 sources")
		return
	}
	if _, err := h.svc.Q.CreateCompetitorSource(r.Context(), database.CreateCompetitorSourceParams{CompetitorID: id, WorkspaceID: ws, Kind: body.Kind, Url: u}); err != nil {
		h.fail(w, err, "add source")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"kind": body.Kind, "url": u}})
}

func (h *CompetitorsHandler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	ws, id, sid := middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id"), chi.URLParam(r, "sourceID")
	if !validUUID(id) || !validUUID(sid) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	n, err := h.svc.Q.DeleteCompetitorSource(r.Context(), database.DeleteCompetitorSourceParams{ID: sid, CompetitorID: id, WorkspaceID: ws})
	if err != nil {
		h.fail(w, err, "remove source")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// People (GET /competitors/{id}/people) lists who is unhappy with them.
func (h *CompetitorsHandler) People(w http.ResponseWriter, r *http.Request) {
	ws, id := middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	if _, ok := h.load(w, r, ws, id); !ok {
		return
	}
	people, err := h.svc.People(r.Context(), ws, id)
	if err != nil {
		h.fail(w, err, "list people")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": people})
}
