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

// crawlTwitterScrapling fetches tweets using the Scrapling stealth browser sidecar.
// Used as a fallback when Pinchtab is unavailable or fails.
// Returns nil if no session is configured for this workspace.
func (m *Monitor) crawlTwitterScrapling(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeTwitter),
	})
	if err != nil {
		return nil
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("twitter-scrapling: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, "x.com")
	if err := m.scrapling.InjectCookies(ctx, cookies); err != nil {
		m.logger.Warn().Err(err).Msg("twitter-scrapling: failed to inject cookies")
		return nil
	}

	searchURL := "https://x.com/search?q=" + url.QueryEscape(kw.Term) + "&f=live&src=typed_query"
	if err := m.scrapling.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("twitter-scrapling: failed to navigate")
		return nil
	}

	select {
	case <-ctx.Done():
		return nil
	case <-time.After(3 * time.Second):
	}

	rawJSON, err := m.scrapling.EvaluateJS(ctx, tweetExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("twitter-scrapling: JS evaluation failed")
		return nil
	}

	var tweets []tweetData
	if err := json.Unmarshal([]byte(rawJSON), &tweets); err != nil {
		m.logger.Warn().Err(err).Msg("twitter-scrapling: failed to parse tweet JSON")
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
			WorkspaceID:   wsID,
			Platform:      string(database.PlatformTypeTwitter),
			PlatformID:    platformID,
			Url:           tweet.URL,
			Content:       tweet.Text,
			AuthorUsername: pgtype.Text{String: author, Valid: author != ""},
			AuthorProfileUrl: pgtype.Text{
				String: "https://x.com/" + author,
				Valid:  author != "",
			},
			PlatformMetadata: jsonBytes(map[string]any{
				"source": "scrapling_search",
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
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("twitter-scrapling: new mentions found")
	}
	return alerts
}
