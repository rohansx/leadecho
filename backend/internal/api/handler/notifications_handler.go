package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type NotificationHandler struct {
	q            *database.Queries
	resendAPIKey string
}

func NewNotificationHandler(q *database.Queries, resendAPIKey string) *NotificationHandler {
	return &NotificationHandler{q: q, resendAPIKey: resendAPIKey}
}

// TestWebhook sends a test notification to verify Slack/Discord/Email channels work.
func (h *NotificationHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Channel    string `json:"channel"`
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if body.Channel == "" {
		writeError(w, http.StatusBadRequest, "channel is required")
		return
	}

	now := time.Now()

	switch body.Channel {
	case "slack":
		if body.WebhookURL == "" {
			writeError(w, http.StatusBadRequest, "webhook_url is required for slack")
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"text": "LeadEcho test notification — your webhook is working!",
		})
		resp, err := http.Post(body.WebhookURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			h.logNotif(r, wsID, "slack", body.WebhookURL, string(payload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "webhook request failed: "+err.Error())
			return
		}
		resp.Body.Close()
		h.logNotif(r, wsID, "slack", body.WebhookURL, string(payload), pgtype.Timestamptz{Time: now, Valid: true})

	case "discord":
		if body.WebhookURL == "" {
			writeError(w, http.StatusBadRequest, "webhook_url is required for discord")
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"content": "LeadEcho test notification — your webhook is working!",
		})
		resp, err := http.Post(body.WebhookURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			h.logNotif(r, wsID, "discord", body.WebhookURL, string(payload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "webhook request failed: "+err.Error())
			return
		}
		resp.Body.Close()
		h.logNotif(r, wsID, "discord", body.WebhookURL, string(payload), pgtype.Timestamptz{Time: now, Valid: true})

	case "email":
		if body.WebhookURL == "" {
			writeError(w, http.StatusBadRequest, "webhook_url (email address) is required")
			return
		}
		if h.resendAPIKey == "" {
			writeError(w, http.StatusBadRequest, "RESEND_API_KEY not configured on the server")
			return
		}
		emailPayload, _ := json.Marshal(map[string]any{
			"from":    "LeadEcho <lead@illuminate.sh>",
			"to":      []string{body.WebhookURL},
			"subject": "LeadEcho Test Email",
			"html":    `<div style="font-family:sans-serif"><h2>LeadEcho</h2><p>This is a test email notification — your email alerts are working!</p></div>`,
		})
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(emailPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+h.resendAPIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			h.logNotif(r, wsID, "email", body.WebhookURL, string(emailPayload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "email send failed: "+err.Error())
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			h.logNotif(r, wsID, "email", body.WebhookURL, string(emailPayload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "Resend API error: "+string(respBody))
			return
		}
		h.logNotif(r, wsID, "email", body.WebhookURL, string(emailPayload), pgtype.Timestamptz{Time: now, Valid: true})

	default:
		writeError(w, http.StatusBadRequest, "channel must be slack, discord, or email")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *NotificationHandler) logNotif(r *http.Request, wsID, channel, recipient, body string, sentAt pgtype.Timestamptz) {
	h.q.CreateNotification(r.Context(), database.CreateNotificationParams{
		WorkspaceID: wsID,
		Channel:     database.NotificationChannel(channel),
		Recipient:   recipient,
		Subject:     pgtype.Text{String: "Test Notification", Valid: true},
		Body:        body,
		Metadata:    []byte("{}"),
		SentAt:      sentAt,
	})
}

// GetWebhookConfig returns the workspace's notification webhook settings.
func (h *NotificationHandler) GetWebhookConfig(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	raw, err := h.q.GetWorkspaceSettings(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		settings = map[string]any{}
	}

	webhooks, _ := settings["webhooks"].(map[string]any)
	if webhooks == nil {
		webhooks = map[string]any{}
	}

	// Tell the frontend whether Resend is configured server-side
	webhooks["resend_configured"] = h.resendAPIKey != ""

	writeJSON(w, http.StatusOK, webhooks)
}

// SaveWebhookConfig saves notification settings to workspace settings.
func (h *NotificationHandler) SaveWebhookConfig(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		SlackURL     string `json:"slack_url"`
		DiscordURL   string `json:"discord_url"`
		EmailTo      string `json:"email_to"`
		Enabled      bool   `json:"enabled"`
		OnNewMention bool   `json:"on_new_mention"`
		OnHighIntent bool   `json:"on_high_intent"`
		OnNewLead    bool   `json:"on_new_lead"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	raw, err := h.q.GetWorkspaceSettings(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		settings = map[string]any{}
	}

	settings["webhooks"] = map[string]any{
		"slack_url":      body.SlackURL,
		"discord_url":    body.DiscordURL,
		"email_to":       body.EmailTo,
		"enabled":        body.Enabled,
		"on_new_mention": body.OnNewMention,
		"on_high_intent": body.OnHighIntent,
		"on_new_lead":    body.OnNewLead,
	}

	data, _ := json.Marshal(settings)
	if err := h.q.UpdateWorkspaceSettings(r.Context(), database.UpdateWorkspaceSettingsParams{
		ID:       wsID,
		Settings: data,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, settings["webhooks"])
}
