package monitor

// competitor watch: a company's own recent posts on X or LinkedIn, and a
// plain-text alert to the workspace's webhooks. These posts are returned to
// the caller and are never stored as mentions.

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"leadecho/internal/treg"
)

// tregLinkedInCompanyPostsTool: $0.002 per success, misses free (catalog
// read 2026-11-15). Text and link only, which is all a move needs.
//
// ponytail: parsed with the generic treg row keys; not yet checked against
// captured traffic. If rows come back empty, capture one response and add
// its keys in treg_parse.go.
const tregLinkedInCompanyPostsTool = "anyapi.linkedin.company_posts_thin"

// ErrTregOff: hosted X / LinkedIn data is not configured on this server.
var ErrTregOff = errors.New("treg is not configured")

// CompanyPost is one post from a company's own account.
type CompanyPost struct {
	ID, Text, URL string
	At            time.Time
}

// CompanyPosts returns the recent posts of an X account ("x") or a LinkedIn
// company page ("linkedin"), given its profile URL. Billing and cap errors
// are returned as they are (billing.ErrNoPaidData and friends).
func (m *Monitor) CompanyPosts(ctx context.Context, wsID, platform, profileURL string) ([]CompanyPost, error) {
	if m == nil || !m.treg.Enabled() {
		return nil, ErrTregOff
	}
	req := treg.Request{WorkspaceID: wsID}
	switch platform {
	case "x":
		u, err := url.Parse(profileURL)
		if err != nil {
			return nil, err
		}
		handle := strings.Trim(u.Path, "/")
		if handle == "" || strings.Contains(handle, "/") {
			return nil, errors.New("not an X profile URL")
		}
		req.Tool, req.Body = tregXUserPostsTool, map[string]any{"username": handle}
	case "linkedin":
		req.Tool, req.Body = tregLinkedInCompanyPostsTool, map[string]any{"url": profileURL}
	default:
		return nil, errors.New("platform must be x or linkedin")
	}
	posts, err := m.tregCall(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]CompanyPost, 0, len(posts))
	for _, p := range posts {
		id := p.ID
		if id == "" {
			id = p.URL
		}
		if id == "" || strings.TrimSpace(p.Text) == "" {
			continue
		}
		out = append(out, CompanyPost{ID: id, Text: p.Text, URL: p.URL, At: p.CreatedAt})
	}
	return out, nil
}

// NotifyText sends one message to the workspace's Slack and Discord
// webhooks, when alerts are on and high-intent alerts are enabled.
func (m *Monitor) NotifyText(ctx context.Context, wsID, slackText, discordText string) {
	cfg, err := m.loadWebhookConfig(ctx, wsID)
	if err != nil {
		m.logger.Error().Err(err).Msg("notify: failed to load webhook config")
		return
	}
	if !cfg.Enabled || !cfg.OnHighIntent {
		return
	}
	now := time.Now()
	if cfg.SlackURL != "" {
		m.sendSlack(ctx, wsID, cfg.SlackURL, slackText, now)
	}
	if cfg.DiscordURL != "" {
		m.sendDiscordText(ctx, wsID, cfg.DiscordURL, discordText, now)
	}
}
