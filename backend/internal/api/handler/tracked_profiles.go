package handler

// enrich (P2 #16): tracked LinkedIn / X profiles CRUD (/api/v1/tracked-profiles).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

const maxTrackedProfiles = 50

type trackedStore interface {
	ListTrackedProfiles(ctx context.Context, workspaceID string) ([]database.TrackedProfile, error)
	CreateTrackedProfile(ctx context.Context, arg database.CreateTrackedProfileParams) (database.TrackedProfile, error)
	DeleteTrackedProfile(ctx context.Context, arg database.DeleteTrackedProfileParams) (int64, error)
}

type TrackedProfilesHandler struct{ q trackedStore }

func NewTrackedProfilesHandler(q *database.Queries) *TrackedProfilesHandler {
	return &TrackedProfilesHandler{q: q}
}

var (
	xHandleRe  = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	liHandleRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,99}$`)
)

// NormalizeTrackedProfile accepts a handle or profile URL and returns the
// handle and canonical profile URL.
func NormalizeTrackedProfile(platform, in string) (string, string, error) {
	in = strings.TrimSpace(in)
	switch platform {
	case "linkedin":
		h := in
		if strings.Contains(in, "/") {
			h = pathAfter(in, []string{"linkedin.com"}, "in")
		}
		if !liHandleRe.MatchString(h) {
			return "", "", errors.New("enter a LinkedIn profile URL (linkedin.com/in/…) or handle")
		}
		return h, "https://www.linkedin.com/in/" + h, nil
	case "x":
		h := strings.TrimPrefix(in, "@")
		if strings.Contains(in, "/") {
			h = pathAfter(in, []string{"x.com", "twitter.com"}, "")
		}
		if !xHandleRe.MatchString(h) {
			return "", "", errors.New("enter an X handle (@name) or profile URL")
		}
		return h, "https://x.com/" + h, nil
	}
	return "", "", errors.New("platform must be linkedin or x")
}

// pathAfter returns the path segment after prefix ("" = first segment) when
// raw is an https URL on one of hosts (or a subdomain).
func pathAfter(raw string, hosts []string, prefix string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	host := strings.TrimPrefix(u.Host, "www.")
	ok := false
	for _, h := range hosts {
		ok = ok || host == h || strings.HasSuffix(host, "."+h)
	}
	if !ok {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if prefix == "" {
		return parts[0]
	}
	if len(parts) >= 2 && parts[0] == prefix {
		return parts[1]
	}
	return ""
}

func (h *TrackedProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListTrackedProfiles(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tracked profiles")
		return
	}
	if rows == nil {
		rows = []database.TrackedProfile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (h *TrackedProfilesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Platform string `json:"platform"`
		Handle   string `json:"handle"` // handle or profile URL
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	handle, profileURL, err := NormalizeTrackedProfile(body.Platform, body.Handle)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	ws := middleware.WorkspaceID(ctx)
	existing, err := h.q.ListTrackedProfiles(ctx, ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tracked profile")
		return
	}
	if len(existing) >= maxTrackedProfiles {
		writeError(w, http.StatusBadRequest, "tracked profile limit reached (50)")
		return
	}
	p, err := h.q.CreateTrackedProfile(ctx, database.CreateTrackedProfileParams{WorkspaceID: ws, Platform: body.Platform, Handle: handle, Url: profileURL})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create tracked profile")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *TrackedProfilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	n, err := h.q.DeleteTrackedProfile(r.Context(), database.DeleteTrackedProfileParams{ID: id, WorkspaceID: middleware.WorkspaceID(r.Context())})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete tracked profile")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "tracked profile not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
