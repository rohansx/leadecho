package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// slackURLAllowed mirrors the Alerts page allowlist: https + hooks.slack.com.
func slackURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "hooks.slack.com")
}

// slackURLFromSettings reads the Slack incoming-webhook URL saved on the
// Alerts page (workspaces.settings.webhooks.slack_url).
func slackURLFromSettings(raw []byte) string {
	var s struct {
		Webhooks struct {
			SlackURL string `json:"slack_url"`
		} `json:"webhooks"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s.Webhooks.SlackURL
}

func postSlack(ctx context.Context, client *http.Client, webhookURL, text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}
