package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// allowedWebhookHosts pins outbound webhook tests to the real provider hosts so
// the endpoint can't be abused as an SSRF probe against internal services.
var allowedWebhookHosts = map[string]map[string]bool{
	"slack": {"hooks.slack.com": true},
	"discord": {
		"discord.com": true, "discordapp.com": true,
		"ptb.discord.com": true, "canary.discord.com": true,
	},
}

// validateWebhookURL enforces https + a provider-specific host allowlist.
func validateWebhookURL(channel, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid webhook URL")
	}
	if u.Scheme != "https" {
		return errors.New("webhook URL must be https")
	}
	hosts := allowedWebhookHosts[channel]
	if !hosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("webhook host not allowed for %s", channel)
	}
	if channel == "discord" && !strings.HasPrefix(u.Path, "/api/webhooks/") {
		return errors.New("invalid discord webhook path")
	}
	return nil
}

// isDisallowedIP blocks loopback/private/link-local/unspecified and CGNAT ranges.
func isDisallowedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// safeWebhookClient resolves the target itself and refuses to connect to any
// internal address (re-checked at dial time to defeat DNS rebinding), and
// disallows redirects.
func safeWebhookClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects are not allowed")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil || len(ips) == 0 {
					return nil, fmt.Errorf("dns lookup failed for %s", host)
				}
				for _, ip := range ips {
					if isDisallowedIP(ip.IP) {
						return nil, fmt.Errorf("blocked internal address: %s", ip.IP)
					}
				}
				// Dial the already-validated IP to avoid a TOCTOU re-resolution.
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

// postWebhook validates + sends a webhook test and treats a non-2xx response as
// a failure (so a broken webhook never reports a green "sent").
func postWebhook(ctx context.Context, channel, webhookURL string, payload []byte) error {
	if err := validateWebhookURL(channel, webhookURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := safeWebhookClient(8 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

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
		if err := validateWebhookURL("slack", body.WebhookURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"text": "LeadEcho test notification — your webhook is working!",
		})
		if err := postWebhook(r.Context(), "slack", body.WebhookURL, payload); err != nil {
			h.logNotif(r, wsID, "slack", body.WebhookURL, string(payload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "webhook test failed: "+err.Error())
			return
		}
		h.logNotif(r, wsID, "slack", body.WebhookURL, string(payload), pgtype.Timestamptz{Time: now, Valid: true})

	case "discord":
		if body.WebhookURL == "" {
			writeError(w, http.StatusBadRequest, "webhook_url is required for discord")
			return
		}
		if err := validateWebhookURL("discord", body.WebhookURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"content": "LeadEcho test notification — your webhook is working!",
		})
		if err := postWebhook(r.Context(), "discord", body.WebhookURL, payload); err != nil {
			h.logNotif(r, wsID, "discord", body.WebhookURL, string(payload), pgtype.Timestamptz{})
			writeError(w, http.StatusBadGateway, "webhook test failed: "+err.Error())
			return
		}
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

	if secret, _ := webhooks["conversion_secret"].(string); secret == "" {
		webhooks["conversion_secret"] = randomWebhookSecret()
		settings["webhooks"] = webhooks
		data, _ := json.Marshal(settings)
		_ = h.q.UpdateWorkspaceSettings(r.Context(), database.UpdateWorkspaceSettingsParams{
			ID:       wsID,
			Settings: data,
		})
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

	existing, _ := settings["webhooks"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	if _, ok := existing["conversion_secret"]; !ok {
		existing["conversion_secret"] = randomWebhookSecret()
	}

	settings["webhooks"] = map[string]any{
		"slack_url":          body.SlackURL,
		"discord_url":        body.DiscordURL,
		"email_to":           body.EmailTo,
		"enabled":            body.Enabled,
		"on_new_mention":     body.OnNewMention,
		"on_high_intent":     body.OnHighIntent,
		"on_new_lead":        body.OnNewLead,
		"conversion_secret":  existing["conversion_secret"],
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

func randomWebhookSecret() string {
	return "whsec_" + randomCode(32)
}
