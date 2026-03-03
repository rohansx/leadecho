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

// linkedInPost is the shape returned by the JavaScript LinkedIn extractor.
type linkedInPost struct {
	Text   string `json:"text"`
	Author string `json:"author"`
	Time   string `json:"time"` // ISO datetime
	URL    string `json:"url"`  // e.g. https://www.linkedin.com/feed/update/urn:li:activity:...
}

// JS snippet that extracts post data from a LinkedIn content search results page.
const linkedInExtractorJS = `JSON.stringify(
	Array.from(document.querySelectorAll('[data-chameleon-result-urn]')).slice(0,20).map(el => ({
		text:   (el.querySelector('.feed-shared-text span') || el.querySelector('[class*="commentary"]') || {innerText:''}).innerText || '',
		author: (el.querySelector('[class*="actor__name"]') || {innerText:''}).innerText || '',
		time:   (el.querySelector('time') || {getAttribute: () => ''}).getAttribute('datetime') || '',
		url:    (el.querySelector('a[href*="/activity/"]') || el.querySelector('a[href*="feed/update"]') || {href:''}).href || ''
	})).filter(p => p.text)
)`

// crawlLinkedInPinchtab fetches LinkedIn posts for a keyword using an authenticated Pinchtab session.
// Returns nil (not empty) if no session is configured.
func (m *Monitor) crawlLinkedInPinchtab(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeLinkedin),
	})
	if err != nil {
		return nil // no session configured
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("linkedin-pinchtab: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, "linkedin.com")
	if err := m.pinchtab.InjectCookies(ctx, cookies); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-pinchtab: failed to inject cookies")
		return nil
	}

	searchURL := "https://www.linkedin.com/search/results/content/?keywords=" +
		url.QueryEscape(kw.Term) + "&sortBy=date_posted"
	if err := m.pinchtab.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("linkedin-pinchtab: failed to navigate")
		return nil
	}

	// LinkedIn renders more slowly than Twitter
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(3 * time.Second):
	}

	rawJSON, err := m.pinchtab.EvaluateJS(ctx, linkedInExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-pinchtab: JS evaluation failed")
		return nil
	}

	var posts []linkedInPost
	if err := json.Unmarshal([]byte(rawJSON), &posts); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-pinchtab: failed to parse post JSON")
		return nil
	}

	var alerts []mentionAlert
	for _, post := range posts {
		if post.Text == "" || post.URL == "" {
			continue
		}
		if !filterContent(post.Text, kw) {
			continue
		}

		platformID := "linkedin_" + extractLinkedInID(post.URL)
		if platformID == "linkedin_" {
			continue
		}

		createdAt := pgtype.Timestamptz{}
		if post.Time != "" {
			if t, err := time.Parse(time.RFC3339, post.Time); err == nil {
				createdAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}

		author := strings.TrimSpace(post.Author)

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:    wsID,
			Platform:       string(database.PlatformTypeLinkedin),
			PlatformID:     platformID,
			Url:            post.URL,
			Content:        post.Text,
			AuthorUsername: pgtype.Text{String: author, Valid: author != ""},
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
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("linkedin-pinchtab: new mentions found")
	}
	return alerts
}

// extractLinkedInID extracts the activity ID from a LinkedIn URL.
// e.g. "https://www.linkedin.com/feed/update/urn:li:activity:1234567890" → "1234567890"
func extractLinkedInID(postURL string) string {
	if idx := strings.LastIndex(postURL, ":"); idx >= 0 {
		id := postURL[idx+1:]
		if qIdx := strings.IndexAny(id, "?#/"); qIdx >= 0 {
			id = id[:qIdx]
		}
		return strings.TrimSpace(id)
	}
	return ""
}
