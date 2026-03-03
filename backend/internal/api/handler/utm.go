package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgconn"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// UTMHandler manages UTM tracking links.
type UTMHandler struct {
	q *database.Queries
}

func NewUTMHandler(q *database.Queries) *UTMHandler {
	return &UTMHandler{q: q}
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
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
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
	_ = h.q.IncrementUTMClicks(r.Context(), code)

	// Build destination with UTM params appended
	dest := buildUTMDestination(link)
	http.Redirect(w, r, dest, http.StatusFound)
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
	dest := link.DestinationUrl
	params := "utm_source=" + link.UtmSource + "&utm_medium=" + link.UtmMedium
	if link.UtmCampaign.Valid {
		params += "&utm_campaign=" + link.UtmCampaign.String
	}
	if link.UtmContent.Valid {
		params += "&utm_content=" + link.UtmContent.String
	}
	if len(dest) > 0 {
		sep := "?"
		for _, c := range dest {
			if c == '?' {
				sep = "&"
				break
			}
		}
		return dest + sep + params
	}
	return dest
}
