package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

var supportedSessionPlatforms = map[string]bool{
	"reddit":   true,
	"twitter":  true,
	"linkedin": true,
}

// SessionResponse is returned for each configured platform session.
type SessionResponse struct {
	Platform         string  `json:"platform"`
	Username         *string `json:"username"`
	IsConfigured     bool    `json:"is_configured"`
	IsPinchtabOnline bool    `json:"is_pinchtab_online"`
}

// SessionHandler manages browser session cookies for Reddit and Twitter.
type SessionHandler struct {
	q        *database.Queries
	encKey   []byte
	pinchtab *browser.PinchtabClient
}

func NewSessionHandler(q *database.Queries, encKey []byte, pinchtab *browser.PinchtabClient) *SessionHandler {
	return &SessionHandler{q: q, encKey: encKey, pinchtab: pinchtab}
}

// List returns session status for all supported platforms.
func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(r.Context()) == nil
	}

	accounts, _ := h.q.ListPlatformSessions(r.Context(), wsID)
	configured := map[string]*database.PlatformAccount{}
	for i := range accounts {
		a := &accounts[i]
		configured[string(a.Platform)] = a
	}

	result := make([]SessionResponse, 0, len(supportedSessionPlatforms))
	for _, platform := range []string{"reddit", "twitter", "linkedin"} {
		resp := SessionResponse{
			Platform:         platform,
			IsPinchtabOnline: pinchtabOnline,
		}
		if acc, ok := configured[platform]; ok && acc.AccessTokenEnc.Valid && acc.AccessTokenEnc.String != "" {
			resp.IsConfigured = true
			if acc.Username.Valid && acc.Username.String != "" {
				u := acc.Username.String
				resp.Username = &u
			}
		}
		result = append(result, resp)
	}

	writeJSON(w, http.StatusOK, result)
}

// Save stores an encrypted session cookie for a platform.
func (h *SessionHandler) Save(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	var body struct {
		SessionCookie string `json:"session_cookie"`
		Username      string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.SessionCookie == "" {
		writeError(w, http.StatusBadRequest, "session_cookie is required")
		return
	}

	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	claims := middleware.ClaimsFromContext(ctx)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	encrypted, err := crypto.Encrypt(h.encKey, body.SessionCookie)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt session")
		return
	}

	meta, _ := json.Marshal(map[string]string{"username": body.Username})

	_, err = h.q.UpsertPlatformSession(ctx, database.UpsertPlatformSessionParams{
		WorkspaceID:    wsID,
		UserID:         userID,
		Platform:       platform,
		Username:       pgtype.Text{String: body.Username, Valid: body.Username != ""},
		AccessTokenEnc: pgtype.Text{String: encrypted, Valid: true},
		Metadata:       meta,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(ctx) == nil
	}

	var usernamePtr *string
	if body.Username != "" {
		u := body.Username
		usernamePtr = &u
	}
	writeJSON(w, http.StatusOK, SessionResponse{
		Platform:         platform,
		Username:         usernamePtr,
		IsConfigured:     true,
		IsPinchtabOnline: pinchtabOnline,
	})
}

// Delete removes a platform session.
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	if err := h.q.DeletePlatformSession(ctx, database.DeletePlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    platform,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(ctx) == nil
	}

	writeJSON(w, http.StatusOK, SessionResponse{
		Platform:         platform,
		IsConfigured:     false,
		IsPinchtabOnline: pinchtabOnline,
	})
}

// Test checks Pinchtab connectivity.
func (h *SessionHandler) Test(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	if h.pinchtab == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"pinchtab_online": false,
			"message":         "Pinchtab not configured — set PINCHTAB_TOKEN to enable",
		})
		return
	}

	if err := h.pinchtab.Heartbeat(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"pinchtab_online": false,
			"message":         err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pinchtab_online": true,
		"message":         "Pinchtab is online",
	})
}
