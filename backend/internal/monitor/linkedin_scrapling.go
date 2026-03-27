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

// crawlLinkedInScrapling fetches LinkedIn posts using the Scrapling stealth browser sidecar.
// Used as a fallback when both Camoufox and Pinchtab are unavailable or fail.
// Returns nil if no session is configured for this workspace.
func (m *Monitor) crawlLinkedInScrapling(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeLinkedin),
	})
	if err != nil {
		return nil
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("linkedin-scrapling: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, "linkedin.com")
	if err := m.scrapling.InjectCookies(ctx, cookies); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-scrapling: failed to inject cookies")
		return nil
	}

	searchURL := "https://www.linkedin.com/search/results/content/?keywords=" +
		url.QueryEscape(kw.Term) + "&sortBy=date_posted"
	if err := m.scrapling.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("linkedin-scrapling: failed to navigate")
		return nil
	}

	// Wait for page render
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(5 * time.Second):
	}

	rawJSON, err := m.scrapling.EvaluateJS(ctx, linkedInExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-scrapling: JS evaluation failed")
		return nil
	}

	var posts []linkedInPost
	if err := json.Unmarshal([]byte(rawJSON), &posts); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-scrapling: failed to parse post JSON")
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
			WorkspaceID:   wsID,
			Platform:      string(database.PlatformTypeLinkedin),
			PlatformID:    platformID,
			Url:           post.URL,
			Content:       post.Text,
			AuthorUsername: pgtype.Text{String: author, Valid: author != ""},
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
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("linkedin-scrapling: new mentions found")
	}
	return alerts
}
