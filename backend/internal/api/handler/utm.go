package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/attribution"
	"leadecho/internal/database"
)

// UTMHandler manages UTM tracking links.
type UTMHandler struct {
	q      *database.Queries
	attrib *attribution.Service
}

func NewUTMHandler(q *database.Queries, attrib *attribution.Service) *UTMHandler {
	return &UTMHandler{q: q, attrib: attrib}
}

// List returns all UTM links for the workspace.
func (h *UTMHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	rows, err := h.q.ListUTMLinksByWorkspace(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list UTM links")
		return
	}
	if rows == nil {
		rows = []database.UtmLink{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// Create generates a new short UTM link.
func (h *UTMHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		DestinationURL string `json:"destination_url"`
		UTMSource      string `json:"utm_source"`
		UTMMedium      string `json:"utm_medium"`
		UTMCampaign    string `json:"utm_campaign"`
		UTMContent     string `json:"utm_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.DestinationURL == "" || body.UTMSource == "" {
		writeError(w, http.StatusBadRequest, "destination_url and utm_source are required")
		return
	}
	// Only http(s) destinations — the /r/{code} redirect is public and
	// unauthenticated, so refuse javascript:/data:/arbitrary schemes that would
	// turn this into an open-redirect / XSS vector.
	if !isHTTPURL(body.DestinationURL) {
		writeError(w, http.StatusBadRequest, "destination_url must be an http(s) URL")
		return
	}

	medium := body.UTMMedium
	if medium == "" {
		medium = "social_reply"
	}

	ctx := r.Context()
	var link database.UtmLink

	// Retry on unique violation (collision unlikely but possible with 8-char codes)
	for attempts := 0; attempts < 5; attempts++ {
		code := randomCode(8)
		var err error
		link, err = h.q.CreateUTMLink(ctx, database.CreateUTMLinkParams{
			WorkspaceID:    wsID,
			Code:           code,
			DestinationUrl: body.DestinationURL,
			UtmSource:      body.UTMSource,
			UtmMedium:      medium,
			UtmCampaign:    pgtype.Text{String: body.UTMCampaign, Valid: body.UTMCampaign != ""},
			UtmContent:     pgtype.Text{String: body.UTMContent, Valid: body.UTMContent != ""},
		})
		if err == nil {
			writeJSON(w, http.StatusCreated, link)
			return
		}
		if !isPgUniqueViolation(err) {
			break
		}
	}

	writeError(w, http.StatusInternalServerError, "failed to create UTM link")
}

// Delete removes a UTM link by id.
func (h *UTMHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid link id")
		return
	}
	_ = h.q.DeleteUTMLink(r.Context(), database.DeleteUTMLinkParams{
		ID:          id,
		WorkspaceID: wsID,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RedirectUTM handles the public short-link redirect.
func (h *UTMHandler) RedirectUTM(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	link, err := h.q.GetUTMLinkByCode(r.Context(), code)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Defense-in-depth: never emit a non-http(s) Location, even if a dangerous
	// destination was somehow persisted before validation existed.
	if !isHTTPURL(link.DestinationUrl) {
		http.NotFound(w, r)
		return
	}
	_ = h.q.IncrementUTMClicks(r.Context(), code)
	_, _ = h.q.CreateUTMEvent(r.Context(), database.CreateUTMEventParams{
		UtmLinkID:    link.ID,
		EventType:    "click",
		Referrer:     pgtype.Text{String: r.Referer(), Valid: r.Referer() != ""},
		UserAgent:    pgtype.Text{String: r.UserAgent(), Valid: r.UserAgent() != ""},
		IpHash:       pgtype.Text{},
		RevenueCents: pgtype.Int4{Int32: 0, Valid: true},
		Metadata:     []byte(`{}`),
	})

	// Build destination with UTM params appended
	dest := buildUTMDestination(link)
	http.Redirect(w, r, dest, http.StatusFound)
}

// RecordConversion records a signup/purchase against a short link and advances
// any linked lead to converted when possible.
// POST /api/v1/utm-links/{code}/conversion
func (h *UTMHandler) RecordConversion(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		EventType    string `json:"event_type"` // signup | purchase
		RevenueCents int32  `json:"revenue_cents"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.EventType == "" {
		body.EventType = "signup"
	}
	if body.EventType != "signup" && body.EventType != "purchase" {
		writeError(w, http.StatusBadRequest, "event_type must be signup or purchase")
		return
	}

	link, err := h.q.GetUTMLinkByCode(r.Context(), code)
	if err != nil || link.WorkspaceID != wsID {
		writeError(w, http.StatusNotFound, "utm link not found")
		return
	}

	updated, err := h.attrib.RecordConversion(r.Context(), wsID, attribution.ConversionRequest{
		UTMCode:      code,
		EventType:    body.EventType,
		RevenueCents: body.RevenueCents,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record conversion")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

const codeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = codeChars[rand.Intn(len(codeChars))]
	}
	return string(b)
}

func isPgUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	pgErr, ok := err.(*pgconn.PgError)
	return ok && pgErr.Code == "23505"
}

func buildUTMDestination(link database.UtmLink) string {
	u, err := url.Parse(link.DestinationUrl)
	if err != nil {
		return link.DestinationUrl
	}
	// Merge UTM params into the existing query with proper percent-encoding so
	// special characters can't inject extra params or corrupt the Location header.
	q := u.Query()
	q.Set("utm_source", link.UtmSource)
	q.Set("utm_medium", link.UtmMedium)
	if link.UtmCampaign.Valid {
		q.Set("utm_campaign", link.UtmCampaign.String)
	}
	if link.UtmContent.Valid {
		q.Set("utm_content", link.UtmContent.String)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
