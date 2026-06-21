package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

// quoraQuestion is the shape returned by the JavaScript Quora extractor.
type quoraQuestion struct {
	Text   string `json:"text"`
	Author string `json:"author"`
	URL    string `json:"url"`
}

// quoraExtractorJS scans the Quora search results DOM for question cards.
// Quora question URLs look like /What-is-the-best-CRM-for-startup (no /question/ prefix).
const quoraExtractorJS = `JSON.stringify(
	Array.from(document.querySelectorAll('a[href^="https://www.quora.com/"]'))
		.filter(a => {
			const href = a.href;
			const text = a.innerText.trim();
			if (!text || text.length < 10) return false;
			const path = new URL(href).pathname;
			if (path === '/' || path === '/following' || path === '/spaces' ||
			    path === '/notifications' || path === '/answer' ||
			    path.startsWith('/search') || path.startsWith('/profile') ||
			    path.startsWith('/topic') || path.startsWith('/space/')) return false;
			if (text.includes('answers') || text === 'Follow' || text === 'Request' ||
			    text === 'Answer' || text.startsWith('Clear')) return false;
			return true;
		})
		.slice(0, 20)
		.map(a => ({
			text:   a.innerText.trim(),
			url:    a.href,
			author: ''
		}))
)`

// crawlQuoraPinchtab fetches Quora questions for a keyword using Pinchtab
// with an authenticated session.
func (m *Monitor) crawlQuoraPinchtab(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeQuora),
	})
	if err != nil {
		return nil
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("quora-pinchtab: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, ".quora.com")

	if err := m.pinchtab.Navigate(ctx, "https://www.quora.com"); err != nil {
		m.logger.Warn().Err(err).Msg("quora-pinchtab: failed to navigate to domain")
		return nil
	}
	if err := m.pinchtab.InjectCookies(ctx, "https://www.quora.com", cookies); err != nil {
		m.logger.Warn().Err(err).Msg("quora-pinchtab: failed to inject cookies")
		return nil
	}

	searchURL := "https://www.quora.com/search?q=" + url.QueryEscape(kw.Term) + "&type=question"
	if err := m.pinchtab.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("quora-pinchtab: failed to navigate")
		return nil
	}

	select {
	case <-ctx.Done():
		return nil
	case <-time.After(4 * time.Second):
	}

	rawJSON, err := m.pinchtab.EvaluateJS(ctx, quoraExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("quora-pinchtab: JS evaluation failed")
		return nil
	}

	var questions []quoraQuestion
	if err := json.Unmarshal([]byte(rawJSON), &questions); err != nil {
		m.logger.Warn().Err(err).Msg("quora-pinchtab: failed to parse question JSON")
		return nil
	}

	var alerts []mentionAlert
	for _, q := range questions {
		text := strings.TrimSpace(q.Text)
		if text == "" {
			continue
		}
		if !filterContent(text, kw) {
			continue
		}

		h := sha256.Sum256([]byte(q.URL))
		platformID := "quora_" + hex.EncodeToString(h[:8])

		author := strings.TrimSpace(q.Author)

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:    wsID,
			Platform:       string(database.PlatformTypeQuora),
			PlatformID:     platformID,
			Url:            q.URL,
			Title:          pgtype.Text{String: truncate(text, 200), Valid: text != ""},
			Content:        text,
			AuthorUsername: pgtype.Text{String: author, Valid: author != ""},
			Status:         database.MentionStatusNew,
			PlatformMetadata: jsonBytes(map[string]any{
				"source": "pinchtab_search",
				"query":  kw.Term,
			}),
			EngagementMetrics: jsonBytes(map[string]any{}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: pgtype.Timestamptz{},
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("quora-pinchtab: new mentions found")
	}
	return alerts
}

// truncate returns s truncated to at most n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
