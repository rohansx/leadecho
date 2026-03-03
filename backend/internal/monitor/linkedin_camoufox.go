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

// crawlLinkedInCamoufox fetches LinkedIn posts using the stealth Camoufox browser sidecar.
// Uses the same JS extractor and cookie injection as the Pinchtab variant.
// Returns nil if no session is configured for this workspace.
func (m *Monitor) crawlLinkedInCamoufox(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
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
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("linkedin-camoufox: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, "linkedin.com")
	if err := m.camoufox.InjectCookies(ctx, cookies); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-camoufox: failed to inject cookies")
		return nil
	}

	searchURL := "https://www.linkedin.com/search/results/content/?keywords=" +
		url.QueryEscape(kw.Term) + "&sortBy=date_posted"
	if err := m.camoufox.Navigate(ctx, searchURL); err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("linkedin-camoufox: failed to navigate")
		return nil
	}

	// Camoufox humanize mode adds its own render delays; wait a bit more for LinkedIn's JS
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(4 * time.Second):
	}

	rawJSON, err := m.camoufox.EvaluateJS(ctx, linkedInExtractorJS)
	if err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-camoufox: JS evaluation failed")
		return nil
	}

	var posts []linkedInPost
	if err := json.Unmarshal([]byte(rawJSON), &posts); err != nil {
		m.logger.Warn().Err(err).Msg("linkedin-camoufox: failed to parse post JSON")
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
				"source": "camoufox_search",
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
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("linkedin-camoufox: new mentions found")
	}
	return alerts
}

