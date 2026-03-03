package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// webhookConfig mirrors the JSON stored in workspace settings.webhooks.
type webhookConfig struct {
	SlackURL     string `json:"slack_url"`
	DiscordURL   string `json:"discord_url"`
	EmailTo      string `json:"email_to"`
	Enabled      bool   `json:"enabled"`
	OnNewMention bool   `json:"on_new_mention"`
	OnHighIntent bool   `json:"on_high_intent"`
	OnNewLead    bool   `json:"on_new_lead"`
}

// mentionAlert is a lightweight summary of a newly-found mention.
type mentionAlert struct {
	ID          string // mention UUID from DB
	WorkspaceID string // for scoring context
	Platform    string
	Keyword     string
	Title       string
	URL         string
	Author      string
	Content     string // for embedding/scoring
}

// notifyNewMentions sends a batched webhook notification for newly discovered mentions.
func (m *Monitor) notifyNewMentions(ctx context.Context, wsID string, alerts []mentionAlert) {
	if len(alerts) == 0 {
		return
	}

	cfg, err := m.loadWebhookConfig(ctx, wsID)
	if err != nil {
		m.logger.Error().Err(err).Msg("notify: failed to load webhook config")
		return
	}

	if !cfg.Enabled || !cfg.OnNewMention {
		return
	}

	// Build a summary message
	summary := fmt.Sprintf("*%d new mention(s) found*\n", len(alerts))
	for i, a := range alerts {
		if i >= 10 {
			summary += fmt.Sprintf("_...and %d more_\n", len(alerts)-10)
			break
		}
		title := a.Title
		if len(title) > 80 {
			title = title[:77] + "..."
		}
		if title == "" {
			title = "(comment)"
		}
		summary += fmt.Sprintf("• [%s] *%s* — <%s|%s> by %s\n", a.Platform, a.Keyword, a.URL, title, a.Author)
	}

	now := time.Now()

	if cfg.SlackURL != "" {
		m.sendSlack(ctx, wsID, cfg.SlackURL, summary, now)
	}
	if cfg.DiscordURL != "" {
		m.sendDiscord(ctx, wsID, cfg.DiscordURL, alerts, now)
	}
	if cfg.EmailTo != "" && m.resendAPIKey != "" {
		m.sendEmail(ctx, wsID, cfg.EmailTo, alerts, now)
	}
}

func (m *Monitor) loadWebhookConfig(ctx context.Context, wsID string) (webhookConfig, error) {
	raw, err := m.q.GetWorkspaceSettings(ctx, wsID)
	if err != nil {
		return webhookConfig{}, err
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return webhookConfig{}, nil // no settings yet
	}

	whRaw, ok := settings["webhooks"]
	if !ok {
		return webhookConfig{}, nil
	}

	var cfg webhookConfig
	if err := json.Unmarshal(whRaw, &cfg); err != nil {
		return webhookConfig{}, nil
	}
	return cfg, nil
}

func (m *Monitor) sendSlack(ctx context.Context, wsID, url, text string, sentAt time.Time) {
	payload, _ := json.Marshal(map[string]string{"text": text})
	if err := m.postWebhook(url, payload); err != nil {
		m.logger.Warn().Err(err).Msg("notify: slack webhook failed")
		m.logNotification(ctx, wsID, database.NotificationChannelSlack, url, text, payload, pgtype.Timestamptz{})
		return
	}
	m.logNotification(ctx, wsID, database.NotificationChannelSlack, url, text, payload, pgtype.Timestamptz{Time: sentAt, Valid: true})
}

func (m *Monitor) sendDiscord(ctx context.Context, wsID, url string, alerts []mentionAlert, sentAt time.Time) {
	// Discord uses markdown-style formatting (no Slack mrkdwn)
	summary := fmt.Sprintf("**%d new mention(s) found**\n", len(alerts))
	for i, a := range alerts {
		if i >= 10 {
			summary += fmt.Sprintf("_...and %d more_\n", len(alerts)-10)
			break
		}
		title := a.Title
		if len(title) > 80 {
			title = title[:77] + "..."
		}
		if title == "" {
			title = "(comment)"
		}
		summary += fmt.Sprintf("• [%s] **%s** — [%s](%s) by %s\n", a.Platform, a.Keyword, title, a.URL, a.Author)
	}

	payload, _ := json.Marshal(map[string]string{"content": summary})
	if err := m.postWebhook(url, payload); err != nil {
		m.logger.Warn().Err(err).Msg("notify: discord webhook failed")
		m.logNotification(ctx, wsID, database.NotificationChannelDiscord, url, summary, payload, pgtype.Timestamptz{})
		return
	}
	m.logNotification(ctx, wsID, database.NotificationChannelDiscord, url, summary, payload, pgtype.Timestamptz{Time: sentAt, Valid: true})
}

func (m *Monitor) sendEmail(ctx context.Context, wsID, to string, alerts []mentionAlert, sentAt time.Time) {
	subject := fmt.Sprintf("LeadEcho: %d new mention(s) found", len(alerts))

	// Build HTML body
	html := `<div style="font-family:sans-serif;max-width:600px">`
	html += fmt.Sprintf(`<h2 style="margin:0 0 16px">%d new mention(s) found</h2>`, len(alerts))
	for i, a := range alerts {
		if i >= 10 {
			html += fmt.Sprintf(`<p style="color:#666;font-style:italic">...and %d more</p>`, len(alerts)-10)
			break
		}
		title := a.Title
		if len(title) > 80 {
			title = title[:77] + "..."
		}
		if title == "" {
			title = "(comment)"
		}
		html += fmt.Sprintf(
			`<div style="border-left:3px solid #6366f1;padding:8px 12px;margin:8px 0">`+
				`<strong>[%s]</strong> %s<br>`+
				`<a href="%s">%s</a> by %s`+
				`</div>`,
			a.Platform, a.Keyword, a.URL, title, a.Author,
		)
	}
	html += `</div>`

	payload, _ := json.Marshal(map[string]any{
		"from":    "LeadEcho <lead@illuminate.sh>",
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		m.logger.Warn().Err(err).Msg("notify: email request build failed")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.resendAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Warn().Err(err).Msg("notify: email send failed")
		m.logNotification(ctx, wsID, database.NotificationChannelEmail, to, html, payload, pgtype.Timestamptz{})
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		m.logger.Warn().Int("status", resp.StatusCode).Msg("notify: resend API error")
		m.logNotification(ctx, wsID, database.NotificationChannelEmail, to, html, payload, pgtype.Timestamptz{})
		return
	}

	m.logNotification(ctx, wsID, database.NotificationChannelEmail, to, html, payload, pgtype.Timestamptz{Time: sentAt, Valid: true})
	m.logger.Info().Str("to", to).Int("count", len(alerts)).Msg("notify: email sent")
}

func (m *Monitor) postWebhook(url string, payload []byte) error {
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

func (m *Monitor) logNotification(ctx context.Context, wsID string, channel database.NotificationChannel, recipient, body string, payload []byte, sentAt pgtype.Timestamptz) {
	m.q.CreateNotification(ctx, database.CreateNotificationParams{
		WorkspaceID: wsID,
		Channel:     channel,
		Recipient:   recipient,
		Subject:     pgtype.Text{String: "New mentions alert", Valid: true},
		Body:        body,
		Metadata:    payload,
		SentAt:      sentAt,
	})
}
