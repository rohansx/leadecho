package monitor

import (
	"context"
	"errors"

	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// Catalog tools (see docs/treg-integration.md for why these).
const (
	tregXTool        = "treg.x.search.posts"          // routed, $0.00075/hit, misses free
	tregLinkedInTool = "anyapi.linkedin.search.posts" // $0.002/success, misses free
	tregRedditTool   = "crawl4ai.reddit.search.posts" // $0.0009/call, misses free
)

// SetTreg enables the hosted X/LinkedIn sources. nil (TREG_TOKEN unset)
// disables X and LinkedIn (Reddit uses its public crawler).
func (m *Monitor) SetTreg(c *treg.Client) { m.treg = c }

// tregSearch runs one treg search. ok=false means "treg could not answer"
// and the caller falls back (Reddit only); errors never reach users.
func (m *Monitor) tregSearch(ctx context.Context, wsID, tool string, body any) ([]tregPost, bool) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tool, Body: body})
	if err != nil {
		// Not allowed / paused / out of credits is expected, not an incident.
		quiet := treg.IsOperatorError(err) || errors.Is(err, billing.ErrNoPaidData) ||
			errors.Is(err, billing.ErrPaidDataPaused) || errors.Is(err, billing.ErrNoCredits)
		if !quiet {
			m.logger.Warn().Err(err).Str("tool", tool).Str("workspace_id", wsID).Msg("treg: search failed, falling back")
		}
		return nil, false
	}
	return parseTregPosts(resp.Body), true
}

// crawlTwitterTreg returns ok=false when treg is off or failed.
func (m *Monitor) crawlTwitterTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	return m.crawlTreg(ctx, wsID, kw, tregXTool, map[string]string{"q": kw.Term}, tregTwitterParams)
}

func (m *Monitor) crawlLinkedInTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	return m.crawlTreg(ctx, wsID, kw, tregLinkedInTool, map[string]string{"query": kw.Term, "datePosted": "last-week"}, tregLinkedInParams)
}

type tregMapper func(string, database.ListActiveKeywordsRow, tregPost) (database.CreateMentionParams, bool)

func (m *Monitor) crawlTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, tool string, body any, toParams tregMapper) ([]mentionAlert, bool) {
	if !m.treg.Enabled() {
		return nil, false
	}
	posts, ok := m.tregSearch(ctx, wsID, tool, body)
	if !ok {
		return nil, false
	}
	var alerts []mentionAlert
	for _, p := range posts {
		if !filterContent(p.Text, kw) {
			continue
		}
		params, ok := toParams(wsID, kw, p)
		if !ok {
			continue
		}
		if a := m.insertMention(ctx, params, kw.Term); a != nil {
			alerts = append(alerts, *a)
		}
	}
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Str("tool", tool).Msg("treg: new mentions found")
	}
	return alerts, true
}

// crawlRedditTreg runs one sitewide search per keyword (newest, past week).
// ponytail: sitewide, not per subreddit: one call instead of one per
// subreddit, and it finds posts outside the suggested subreddits.
func (m *Monitor) crawlRedditTreg(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) ([]mentionAlert, bool) {
	body := map[string]any{"query": kw.Term, "sort": "new", "t": "week", "limit": 25}
	return m.crawlTreg(ctx, wsID, kw, tregRedditTool, body, tregRedditParams)
}

// redditViaTreg: the official Reddit API (free) wins when configured; else
// treg, ahead of the unauthenticated public crawler.
func (m *Monitor) redditViaTreg() bool {
	return m.treg.Enabled() && m.sources().RedditOAuth == nil
}
