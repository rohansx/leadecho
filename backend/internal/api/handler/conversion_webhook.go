package handler

import (
	"encoding/json"
	"net/http"

	"leadecho/internal/attribution"
	"leadecho/internal/database"
)

// ConversionWebhookHandler accepts signup/purchase events from external systems.
type ConversionWebhookHandler struct {
	q      *database.Queries
	attrib *attribution.Service
}

func NewConversionWebhookHandler(q *database.Queries, attrib *attribution.Service) *ConversionWebhookHandler {
	return &ConversionWebhookHandler{q: q, attrib: attrib}
}

// POST /api/v1/hooks/conversion
func (h *ConversionWebhookHandler) RecordConversion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID  string         `json:"workspace_id"`
		Secret       string         `json:"secret"`
		UTMCode      string         `json:"utm_code"`
		EventType    string         `json:"event_type"`
		RevenueCents int32          `json:"revenue_cents"`
		Metadata     map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.WorkspaceID == "" || body.UTMCode == "" || body.Secret == "" {
		writeError(w, http.StatusBadRequest, "workspace_id, utm_code, and secret are required")
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

	link, err := h.attrib.RecordConversion(r.Context(), body.WorkspaceID, attribution.ConversionRequest{
		UTMCode:      body.UTMCode,
		EventType:    body.EventType,
		RevenueCents: body.RevenueCents,
		Metadata:     body.Metadata,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "conversion not recorded")
		return
	}
	writeJSON(w, http.StatusOK, link)
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
	return stored != "" && stored == secret
}
