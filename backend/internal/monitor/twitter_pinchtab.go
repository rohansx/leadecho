package monitor

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

// tweetData is the shape returned by the JavaScript tweet extractor.
type tweetData struct {
	Text   string `json:"text"`
	Author string `json:"author"`
	Time   string `json:"time"`   // ISO datetime
	URL    string `json:"url"`    // e.g. https://x.com/user/status/12345
}

// JS snippet that extracts tweet data from the current Twitter/X search page.
const tweetExtractorJS = `JSON.stringify(
	Array.from(document.querySelectorAll('[data-testid="tweet"]')).slice(0,20).map(t => ({
		text:   (t.querySelector('[data-testid="tweetText"]') || {innerText:''}).innerText || '',
		author: (t.querySelector('[data-testid="User-Name"] span') || {innerText:''}).innerText || '',
		time:   (t.querySelector('time') || {}).getAttribute ? t.querySelector('time').getAttribute('datetime') || '' : '',
		url:    (t.querySelector('a[href*="/status/"]') || {href:''}).href || ''
	}))
)`

// crawlTwitterPinchtab fetches tweets for a keyword using an authenticated Pinchtab session.
// Returns nil (not empty) if no session is configured.
func (m *Monitor) crawlTwitterPinchtab(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	// Load session cookie for this workspace
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeTwitter),
	})
	if err != nil {
		return nil // no session configured
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	// Decrypt session cookie
	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("twitter-pinchtab: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, "x.com")

	if err := m.pinchtab.InjectCookies(ctx, cookies); err != nil {
		m.logger.Warn().Err(err).Msg("twitter-pinchtab: failed to inject cookies")
		return nil
	}

	searchURL := "https://x.com/search?q=" + url.QueryEscape(kw.Term) + "&f=live&src=typed_query"
	if err := m.pinchtab.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("twitter-pinchtab: failed to navigate")
		return nil
	}

	// Human-like wait for page to render
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(2 * time.Second):
	}

	rawJSON, err := m.pinchtab.EvaluateJS(ctx, tweetExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("twitter-pinchtab: JS evaluation failed")
		return nil
	}

	var tweets []tweetData
	if err := json.Unmarshal([]byte(rawJSON), &tweets); err != nil {
		m.logger.Warn().Err(err).Msg("twitter-pinchtab: failed to parse tweet JSON")
		return nil
	}

	var alerts []mentionAlert
	for _, tweet := range tweets {
		if tweet.Text == "" || tweet.URL == "" {
			continue
		}
		if !filterContent(tweet.Text, kw) {
			continue
		}

		platformID := "twitter_" + extractStatusID(tweet.URL)
		if platformID == "twitter_" {
			continue
		}

		createdAt := pgtype.Timestamptz{}
		if tweet.Time != "" {
			if t, err := time.Parse(time.RFC3339, tweet.Time); err == nil {
				createdAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}

		author := strings.TrimPrefix(tweet.Author, "@")

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:    wsID,
			Platform:       string(database.PlatformTypeTwitter),
			PlatformID:     platformID,
			Url:            tweet.URL,
			Content:        tweet.Text,
			AuthorUsername: pgtype.Text{String: author, Valid: author != ""},
			AuthorProfileUrl: pgtype.Text{
				String: "https://x.com/" + author,
				Valid:  author != "",
			},
			PlatformMetadata: jsonBytes(map[string]any{
				"source": "pinchtab_search",
				"query":  kw.Term,
			}),
			EngagementMetrics: jsonBytes(map[string]any{}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: createdAt,
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("twitter-pinchtab: new mentions found")
	}
	return alerts
}

// extractStatusID extracts the numeric status ID from a Twitter URL.
// e.g. "https://x.com/user/status/1234567890" → "1234567890"
func extractStatusID(tweetURL string) string {
	parts := strings.Split(tweetURL, "/status/")
	if len(parts) < 2 {
		return ""
	}
	// Trim any query parameters or path suffix
	id := parts[1]
	if idx := strings.IndexAny(id, "?#/"); idx >= 0 {
		id = id[:idx]
	}
	return strings.TrimSpace(id)
}
