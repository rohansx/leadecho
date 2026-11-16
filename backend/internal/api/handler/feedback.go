package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/lobby"
)

// Lobby feedback widget: the dashboard loads Lobby's fb.js for people Lobby
// lets give feedback (claimed prospects, and testers added in Lobby). The
// server asks Lobby for each user's reporter token with the secret key; the
// browser only ever sees the public key and that user's own token.

type feedbackLobby interface {
	ReporterToken(ctx context.Context, email string) (string, error)
	PublicKey(ctx context.Context) (string, error)
}

type cachedReporter struct {
	token string // "" = feedback off for them
	until time.Time
}

const (
	reporterTTL    = 24 * time.Hour // tokens last 90 days; re-ask daily
	feedbackOffTTL = 10 * time.Minute
)

type FeedbackHandler struct {
	lobby  feedbackLobby
	logger zerolog.Logger

	// ponytail: per-process cache, a few hundred users at most. Lobby's
	// on/off switch is also checked by the widget itself on every page load.
	mu        sync.Mutex
	publicKey string
	reporters map[string]cachedReporter
}

// NewFeedbackHandler returns nil when Lobby isn't configured.
func NewFeedbackHandler(l *lobby.Client, logger zerolog.Logger) *FeedbackHandler {
	if l == nil {
		return nil
	}
	return &FeedbackHandler{lobby: l, logger: logger, reporters: map[string]cachedReporter{}}
}

type feedbackWidget struct {
	Enabled  bool   `json:"enabled"`
	Project  string `json:"project,omitempty"`
	Reporter string `json:"reporter,omitempty"`
}

// Widget tells the dashboard whether to load the feedback widget for the
// signed-in user, and with which keys. Any Lobby trouble just hides it.
func (h *FeedbackHandler) Widget(w http.ResponseWriter, r *http.Request) {
	off := feedbackWidget{}
	if h == nil {
		writeJSON(w, http.StatusOK, off)
		return
	}
	c := middleware.ClaimsFromContext(r.Context())
	if c == nil || c.Email == "" {
		writeJSON(w, http.StatusOK, off)
		return
	}
	email := strings.ToLower(strings.TrimSpace(c.Email))
	pk, err := h.project(r.Context())
	if err != nil {
		h.logger.Warn().Err(err).Msg("feedback: lobby public key")
		writeJSON(w, http.StatusOK, off)
		return
	}
	tok := h.reporter(r.Context(), email)
	if tok == "" {
		writeJSON(w, http.StatusOK, off)
		return
	}
	writeJSON(w, http.StatusOK, feedbackWidget{Enabled: true, Project: pk, Reporter: tok})
}

func (h *FeedbackHandler) project(ctx context.Context) (string, error) {
	h.mu.Lock()
	pk := h.publicKey
	h.mu.Unlock()
	if pk != "" {
		return pk, nil
	}
	pk, err := h.lobby.PublicKey(ctx)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	h.publicKey = pk
	h.mu.Unlock()
	return pk, nil
}

// reporter returns the user's token, or "" when they get no widget.
func (h *FeedbackHandler) reporter(ctx context.Context, email string) string {
	now := time.Now()
	h.mu.Lock()
	cr, ok := h.reporters[email]
	h.mu.Unlock()
	if ok && now.Before(cr.until) {
		return cr.token
	}
	tok, err := h.lobby.ReporterToken(ctx, email)
	switch {
	case errors.Is(err, lobby.ErrFeedbackOff):
		cr = cachedReporter{until: now.Add(feedbackOffTTL)}
	case err != nil:
		h.logger.Warn().Err(err).Msg("feedback: lobby reporter token")
		return "" // not cached: try again on the next page load
	default:
		cr = cachedReporter{token: tok, until: now.Add(reporterTTL)}
	}
	h.mu.Lock()
	h.reporters[email] = cr
	h.mu.Unlock()
	return cr.token
}
