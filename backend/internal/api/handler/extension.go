package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

var validExtensionPlatforms = map[string]bool{
	"reddit":       true,
	"twitter":      true,
	"linkedin":     true,
	"hackernews":   true,
	"devto":        true,
	"lobsters":     true,
	"indiehackers": true,
}

// ExtensionHandler manages extension tokens and handles signal ingestion from the extension.
type ExtensionHandler struct {
	q   *database.Queries
	mon *monitor.Monitor
}

func NewExtensionHandler(q *database.Queries, mon *monitor.Monitor) *ExtensionHandler {
	return &ExtensionHandler{q: q, mon: mon}
}

// ── Token management (JWT-protected, called from the dashboard) ───────────────

type extensionTokenResponse struct {
	HasToken    bool       `json:"has_token"`
	MaskedToken string     `json:"masked_token,omitempty"`
	Name        string     `json:"name,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

// GetToken returns the current token info for the workspace (token value is masked).
func (h *ExtensionHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	row, err := h.q.GetExtensionTokenByWorkspace(r.Context(), wsID)
	if err != nil {
		writeJSON(w, http.StatusOK, extensionTokenResponse{HasToken: false})
		return
	}

	resp := extensionTokenResponse{
		HasToken:    true,
		MaskedToken: maskToken(row.Token),
		Name:        row.Name,
		CreatedAt:   &row.CreatedAt,
	}
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		resp.LastUsedAt = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

// RotateToken generates a new token, replacing any existing one.
// The full token value is returned once — it will not be retrievable again.
func (h *ExtensionHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = "Default"
	}

	token, err := generateExtensionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	ctx := r.Context()
	_ = h.q.DeleteExtensionTokenByWorkspace(ctx, wsID)

	row, err := h.q.CreateExtensionToken(ctx, database.CreateExtensionTokenParams{
		WorkspaceID: wsID,
		Token:       token,
		Name:        body.Name,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":      row.Token,
		"name":       row.Name,
		"created_at": row.CreatedAt,
	})
}

// RevokeToken deletes the workspace's extension token.
func (h *ExtensionHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	_ = h.q.DeleteExtensionTokenByWorkspace(r.Context(), wsID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ── Signal ingestion (X-Extension-Key protected) ─────────────────────────────

type signalPayload struct {
	PlatformID string `json:"platform_id"`
	Platform   string `json:"platform"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Author     string `json:"author"`
	AuthorURL  string `json:"author_url"`
}

type ingestRequest struct {
	Signals []signalPayload `json:"signals"`
}

// IngestSignals accepts a batch of signals from the extension, inserts them as mentions,
// and triggers the 4-stage scoring pipeline asynchronously.
func (h *ExtensionHandler) IngestSignals(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.ExtensionWorkspaceID(r.Context())
	if wsID == "" {
		writeError(w, http.StatusUnauthorized, "no workspace")
		return
	}

	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Signals) == 0 {
		writeJSON(w, http.StatusOK, map[string]int{"inserted": 0})
		return
	}
	if len(req.Signals) > 50 {
		writeError(w, http.StatusBadRequest, "max 50 signals per batch")
		return
	}

	ctx := r.Context()
	var alerts []monitor.SignalAlert

	for _, s := range req.Signals {
		if s.PlatformID == "" || s.URL == "" || s.Content == "" {
			continue
		}
		if !validExtensionPlatforms[s.Platform] {
			continue
		}

		mention, err := h.q.CreateMention(ctx, database.CreateMentionParams{
			WorkspaceID:           wsID,
			KeywordID:             pgtype.UUID{Valid: false},
			Platform:              s.Platform,
			PlatformID:            s.PlatformID,
			Url:                   s.URL,
			Title:                 pgtype.Text{String: s.Title, Valid: s.Title != ""},
			Content:               s.Content,
			AuthorUsername:        pgtype.Text{String: s.Author, Valid: s.Author != ""},
			AuthorProfileUrl:      pgtype.Text{String: s.AuthorURL, Valid: s.AuthorURL != ""},
			AuthorKarma:           pgtype.Int4{},
			AuthorAccountAgeDays:  pgtype.Int4{},
			RelevanceScore:        pgtype.Float4{},
			Intent:                database.NullIntentType{},
			ConversionProbability: pgtype.Float4{},
			Status:                database.MentionStatusNew,
			PlatformMetadata:      []byte(`{"source":"extension"}`),
			EngagementMetrics:     []byte(`{}`),
			KeywordMatches:        []string{},
			PlatformCreatedAt:     pgtype.Timestamptz{},
		})
		if err != nil {
			if extensionIsDuplicate(err) {
				continue
			}
			continue
		}

		alerts = append(alerts, monitor.SignalAlert{
			ID:       mention.ID,
			Platform: s.Platform,
			Title:    s.Title,
			URL:      s.URL,
			Author:   s.Author,
			Content:  s.Content,
		})
	}

	if len(alerts) > 0 && h.mon != nil {
		go h.mon.IngestSignals(context.Background(), wsID, alerts)
	}

	writeJSON(w, http.StatusOK, map[string]int{"inserted": len(alerts)})
}

// ── Extension data endpoints (X-Extension-Key protected) ─────────────────────

// ListMentions returns recent high-signal mentions for the extension side panel.
func (h *ExtensionHandler) ListMentions(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.ExtensionWorkspaceID(r.Context())
	if wsID == "" {
		writeError(w, http.StatusUnauthorized, "no workspace")
		return
	}

	rows, err := h.q.ListRecentLeadsForWorkspace(r.Context(), database.ListRecentLeadsForWorkspaceParams{
		WorkspaceID: wsID,
		Lim:         50,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list mentions")
		return
	}
	if rows == nil {
		rows = []database.ListRecentLeadsForWorkspaceRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// GetReplyQueue returns approved replies waiting to be posted.
func (h *ExtensionHandler) GetReplyQueue(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.ExtensionWorkspaceID(r.Context())
	if wsID == "" {
		writeError(w, http.StatusUnauthorized, "no workspace")
		return
	}

	rows, err := h.q.ListApprovedRepliesByWorkspace(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list reply queue")
		return
	}
	if rows == nil {
		rows = []database.ListApprovedRepliesByWorkspaceRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// MarkReplyPosted marks an approved reply as posted after the extension has submitted it.
func (h *ExtensionHandler) MarkReplyPosted(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.ExtensionWorkspaceID(r.Context())
	if wsID == "" {
		writeError(w, http.StatusUnauthorized, "no workspace")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}

	reply, err := h.q.MarkReplyPosted(r.Context(), database.MarkReplyPostedParams{
		ID:          id,
		WorkspaceID: wsID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "reply not found or not owned by workspace")
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func generateExtensionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:8] + "..." + token[len(token)-4:]
}

func extensionIsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return extContains(msg, "duplicate key") || extContains(msg, "unique constraint")
}

func extContains(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
