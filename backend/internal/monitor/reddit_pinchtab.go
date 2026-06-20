package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/browser"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

// crawlRedditPinchtab fetches new posts using authenticated Pinchtab browser session.
// Returns nil (not empty slice) if there's no session configured or on Pinchtab error,
// allowing the caller to fall back to the unauthenticated crawler.
func (m *Monitor) crawlRedditPinchtab(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	if len(kw.Subreddits) == 0 {
		return nil
	}

	// Load session cookie for this workspace
	session, err := m.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    string(database.PlatformTypeReddit),
	})
	if err != nil {
		return nil // no session configured — signal to use fallback
	}
	if !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return nil
	}

	// Decrypt the session cookie
	cookieStr, err := crypto.Decrypt(m.encKey, session.AccessTokenEnc.String)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace", wsID).Msg("reddit-pinchtab: failed to decrypt session")
		return nil
	}

	cookies := parseCookieString(cookieStr, ".reddit.com")

	// Navigate to reddit.com and inject cookies ONCE for all subreddits.
	// Doing this per-subreddit causes too many page loads and triggers Reddit's
	// anti-bot rate limiting.
	if err := m.pinchtab.Navigate(ctx, "https://www.reddit.com"); err != nil {
		m.logger.Warn().Err(err).Msg("reddit-pinchtab: failed to navigate to domain")
		return nil
	}
	if err := m.pinchtab.InjectCookies(ctx, "https://www.reddit.com", cookies); err != nil {
		m.logger.Warn().Err(err).Msg("reddit-pinchtab: failed to inject cookies")
		return nil
	}

	var alerts []mentionAlert
	var failCount int

	for _, sub := range kw.Subreddits {
		results, err := m.fetchSubredditPinchtab(ctx, wsID, kw, sub)
		if err != nil {
			m.logger.Warn().Err(err).Str("subreddit", sub).Msg("reddit-pinchtab: subreddit failed, skipping")
			failCount++
			// If 3+ subreddits fail in a row, Reddit is likely rate-limiting.
			// Return what we have so far instead of falling back to unauthenticated.
			if failCount >= 3 {
				m.logger.Warn().Msg("reddit-pinchtab: too many failures, stopping subreddit loop")
				break
			}
			continue
		}
		alerts = append(alerts, results...)

		select {
		case <-ctx.Done():
			return alerts
		case <-time.After(3 * time.Second):
		}
	}

	// Return whatever we collected — only signal fallback if we got nothing.
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("reddit-pinchtab: new mentions found")
		return alerts
	}
	return nil // no data collected — signal caller to try fallback
}

func (m *Monitor) fetchSubredditPinchtab(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, sub string) ([]mentionAlert, error) {
	jsonURL := fmt.Sprintf("https://www.reddit.com/r/%s/new.json?limit=25", sub)

	// Cookies are already injected by the caller; just navigate to the JSON endpoint.
	if err := m.pinchtab.Navigate(ctx, jsonURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Wait for page to load before extracting text.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(2 * time.Second):
	}

	text, err := m.pinchtab.GetText(ctx)
	if err != nil {
		return nil, fmt.Errorf("get text: %w", err)
	}

	// The authenticated JSON API returns the same redditListing structure
	var listing redditListing
	if err := json.Unmarshal([]byte(text), &listing); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	var alerts []mentionAlert
	for _, child := range listing.Data.Children {
		post := child.Data
		if post.Author == "[deleted]" || post.Author == "" || post.Author == "AutoModerator" {
			continue
		}

		content := post.Selftext
		if content == "" {
			content = post.Title
		}
		if !filterContent(content, kw) && !filterContent(post.Title, kw) {
			continue
		}

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:    wsID,
			KeywordID:      pgtype.UUID{},
			Platform:       string(database.PlatformTypeReddit),
			PlatformID:     "reddit_" + post.ID,
			Url:            "https://reddit.com" + post.Permalink,
			Title:          pgtype.Text{String: post.Title, Valid: post.Title != ""},
			Content:        content,
			AuthorUsername: pgtype.Text{String: post.Author, Valid: true},
			AuthorProfileUrl: pgtype.Text{
				String: "https://reddit.com/u/" + post.Author,
				Valid:  true,
			},
			PlatformMetadata: jsonBytes(map[string]any{
				"subreddit": sub,
				"score":     post.Score,
				"source":    "pinchtab",
			}),
			EngagementMetrics: jsonBytes(map[string]any{
				"score":        post.Score,
				"num_comments": post.NumComments,
			}),
			KeywordMatches:   []string{kw.Term},
			PlatformCreatedAt: pgtype.Timestamptz{
				Time:  time.Unix(int64(post.CreatedUTC), 0),
				Valid: true,
			},
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return alerts, nil
}

// parseCookieString parses a cookie header string ("name=value; name2=value2")
// into browser.Cookie structs with the given domain.
func parseCookieString(cookieStr, domain string) []browser.Cookie {
	var cookies []browser.Cookie
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if name == "" {
			continue
		}
		cookies = append(cookies, browser.Cookie{
			Name:   name,
			Value:  value,
			Domain: domain,
			Path:   "/",
		})
	}
	return cookies
}
