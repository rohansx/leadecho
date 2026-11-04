package monitor

import (
	"context"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// Catalog tools (see docs/treg-integration.md for why these).
const (
	tregXTool        = "treg.x.search.posts"          // routed, $0.00075/hit, misses free
	tregLinkedInTool = "anyapi.linkedin.search.posts" // $0.002/success, misses free
)

// SetTreg enables the hosted X/LinkedIn sources. nil (TREG_TOKEN unset)
// keeps the Pinchtab/Camoufox/Scrapling paths.
func (m *Monitor) SetTreg(c *treg.Client) { m.treg = c }

// tregSearch runs one treg search. ok=false means "treg could not answer"
// and the caller falls back to the browser paths; errors never reach users.
func (m *Monitor) tregSearch(ctx context.Context, wsID, tool string, body any) ([]tregPost, bool) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tool, Body: body})
	if err != nil {
		if !treg.IsOperatorError(err) { // operator errors are already alerted by the client
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
