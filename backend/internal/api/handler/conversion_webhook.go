package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"leadecho/internal/api/middleware"
	"leadecho/internal/attribution"
	"leadecho/internal/database"
)

// ConversionWebhookHandler accepts signup/purchase events from external systems.
type ConversionWebhookHandler struct {
	q       *database.Queries
	attrib  *attribution.Service
	wsQuota *middleware.Quota // legacy hook, per workspace_id; nil = unlimited
}

func NewConversionWebhookHandler(q *database.Queries, attrib *attribution.Service, wsQuota *middleware.Quota) *ConversionWebhookHandler {
	return &ConversionWebhookHandler{q: q, attrib: attrib, wsQuota: wsQuota}
}

const (
	maxConversionBody    = 8 << 10
	maxConversionRevenue = 10_000_000 // cents: 100k per event; anything above is a bug or abuse
	maxConversionEventID = 128
)

var utmCodeRE = regexp.MustCompile(`^[A-Za-z0-9]{4,32}$`)

// conversionInput is what the customer's site (server side) reports. The
// visitor is identified by the tracked link's code or by utm_content, which
// the /r/{code} redirect sets to the reply id on the landing URL.
type conversionInput struct {
	UTMCode      string         `json:"utm_code"`
	UTMContent   string         `json:"utm_content"`
	EventType    string         `json:"event_type"` // signup (default) | purchase
	RevenueCents int32          `json:"revenue_cents"`
	EventID      string         `json:"event_id"` // optional idempotency key
	Metadata     map[string]any `json:"metadata"`
}

// validate normalizes and checks the input; the error text is safe to return.
func (c conversionInput) validate() (conversionInput, error) {
	c.UTMCode = strings.TrimSpace(c.UTMCode)
	c.UTMContent = strings.TrimSpace(c.UTMContent)
	c.EventID = strings.TrimSpace(c.EventID)
	c.EventType = strings.ToLower(strings.TrimSpace(c.EventType))
	switch {
	case c.UTMCode == "" && c.UTMContent == "":
		return c, errors.New("utm_code or utm_content is required")
	case c.UTMCode != "" && !utmCodeRE.MatchString(c.UTMCode):
		return c, errors.New("utm_code is malformed")
	case c.UTMCode == "" && !parseUUID(c.UTMContent).Valid:
		return c, errors.New("utm_content must be the reply id from the landing URL")
	}
	if c.EventType == "" {
		c.EventType = "signup"
	}
	if c.EventType != "signup" && c.EventType != "purchase" {
		return c, errors.New("event_type must be signup or purchase")
	}
	if c.RevenueCents < 0 || c.RevenueCents > maxConversionRevenue {
		return c, errors.New("revenue_cents out of range")
	}
	if len(c.EventID) > maxConversionEventID {
		return c, errors.New("event_id too long")
	}
	return c, nil
}

// decodeConversion reads a size-capped JSON body into v.
func decodeConversion(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxConversionBody)
	return json.NewDecoder(r.Body).Decode(v)
}

// POST /api/public/v1/conversions — workspace API key with the write scope
// (rate-limited by APIKeyAuth). The recommended way to report sign-ups.
func (h *ConversionWebhookHandler) RecordConversionAPI(w http.ResponseWriter, r *http.Request) {
	wsID, _ := middleware.APIKeyPrincipal(r.Context())
	if wsID == "" {
		writeError(w, http.StatusUnauthorized, "API key required")
		return
	}
	var in conversionInput
	if err := decodeConversion(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in, err := in.validate()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	link, status, msg := h.resolveLink(r, wsID, in)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	updated, err := h.attrib.RecordConversion(r.Context(), wsID, attribution.ConversionRequest{
		UTMCode: link.Code, EventType: in.EventType, RevenueCents: in.RevenueCents,
		Metadata: withEventID(in.Metadata, in.EventID),
	})
	if errors.Is(err, attribution.ErrDuplicateConversion) {
		writeJSON(w, http.StatusOK, map[string]any{"recorded": false, "duplicate": true})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record conversion")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recorded": true, "event_type": in.EventType, "signups": updated.SignupCount,
		"reply_id": updated.UtmContent.String,
	})
}

// resolveLink finds the workspace's tracked link by code, or by the reply id
// (utm_content) through the reply's current link. status != 0 is an error.
func (h *ConversionWebhookHandler) resolveLink(r *http.Request, wsID string, in conversionInput) (database.UtmLink, int, string) {
	ctx := r.Context()
	if in.UTMCode != "" {
		link, err := h.q.GetUTMLinkByCode(ctx, in.UTMCode)
		if err != nil || link.WorkspaceID != wsID {
			return database.UtmLink{}, http.StatusNotFound, "tracked link not found"
		}
		return link, 0, ""
	}
	reply, err := h.q.GetReply(ctx, database.GetReplyParams{ID: in.UTMContent, WorkspaceID: wsID})
	if err != nil || !reply.UtmLinkID.Valid {
		return database.UtmLink{}, http.StatusNotFound, "no tracked reply for utm_content"
	}
	link, err := h.q.GetUTMLinkByID(ctx, database.GetUTMLinkByIDParams{ID: formatUUID(reply.UtmLinkID), WorkspaceID: wsID})
	if err != nil {
		return database.UtmLink{}, http.StatusNotFound, "no tracked reply for utm_content"
	}
	return link, 0, ""
}

// withEventID returns a copy of meta carrying the idempotency key.
func withEventID(meta map[string]any, eventID string) map[string]any {
	out := make(map[string]any, len(meta)+1)
	for k, v := range meta {
		out[k] = v
	}
	if eventID != "" {
		out["event_id"] = eventID
	}
	return out
}

// POST /api/v1/hooks/conversion — legacy: workspace id + conversion secret in
// the body. Same validation as the API-key endpoint.
func (h *ConversionWebhookHandler) RecordConversion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Secret      string `json:"secret"`
		conversionInput
	}
	if err := decodeConversion(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.WorkspaceID == "" || body.Secret == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and secret are required")
		return
	}
	// Per-workspace cap before the secret check, so guessing one workspace's
	// secret from many IPs is throttled too (the route also caps per IP).
	if h.wsQuota != nil && !h.wsQuota.Allow(r.Context(), body.WorkspaceID) {
		writeError(w, http.StatusTooManyRequests, "too many requests for this workspace, try again shortly")
		return
	}
	in, err := body.conversionInput.validate()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ws, err := h.q.GetWorkspace(r.Context(), body.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !webhookSecretMatches(ws.Settings, body.Secret) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	link, status, msg := h.resolveLink(r, body.WorkspaceID, in)
	if status != 0 {
		writeError(w, status, msg)
		return
	}

	updated, err := h.attrib.RecordConversion(r.Context(), body.WorkspaceID, attribution.ConversionRequest{
		UTMCode:      link.Code,
		EventType:    in.EventType,
		RevenueCents: in.RevenueCents,
		Metadata:     withEventID(in.Metadata, in.EventID),
	})
	if errors.Is(err, attribution.ErrDuplicateConversion) {
		writeJSON(w, http.StatusOK, map[string]any{"recorded": false, "duplicate": true})
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "conversion not recorded")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func webhookSecretMatches(settings []byte, secret string) bool {
	var s map[string]any
	if err := json.Unmarshal(settings, &s); err != nil {
		return false
	}
	wh, _ := s["webhooks"].(map[string]any)
	if wh == nil {
		return false
	}
	stored, _ := wh["conversion_secret"].(string)
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(secret)) == 1
}
