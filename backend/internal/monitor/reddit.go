package monitor

import (
	"context"
	"errors"
	"net/http"
	"time"

	"leadecho/internal/database"
)

// Reddit JSON API response structures (no auth required for public subreddits).
type redditListing struct {
	Data struct {
		Children []struct {
			Data redditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type redditPost struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Selftext    string  `json:"selftext"`
	Author      string  `json:"author"`
	Subreddit   string  `json:"subreddit"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
}

// crawlReddit fetches new posts from each configured subreddit and filters
// them locally by keyword. This avoids Reddit's aggressive search API rate limits.
//
// During a backfill (since set on ctx) a keyword without subreddits falls back
// to one site-wide search, so a freshly onboarded listener is not empty on
// Reddit. Regular ticks keep skipping it.
func (m *Monitor) crawlReddit(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	since := sinceFrom(ctx)
	if len(kw.Subreddits) == 0 && since.IsZero() {
		return nil // no subreddits configured — skip
	}

	// If Reddit recently returned 403, the public JSON API is blocked.
	// Skip all subreddit fetches until the backoff window expires instead of
	// hammering Reddit and generating a wall of 403 logs every tick.
	if m.redditBackedOff() {
		return nil
	}

	if len(kw.Subreddits) == 0 {
		posts, err := m.sources().SearchReddit(ctx, kw.Term, since)
		if m.handleRedditErr(err, "search") {
			return nil
		}
		return m.insertPosts(ctx, wsID, kw, posts)
	}

	var alerts []mentionAlert
	for _, sub := range kw.Subreddits {
		results := m.fetchSubreddit(ctx, wsID, kw, sub)
		alerts = append(alerts, results...)

		// If fetchSubreddit triggered a 403 backoff, stop hitting more subreddits.
		if m.redditBackedOff() {
			break
		}

		// Pause between subreddit requests to stay under rate limits
		select {
		case <-ctx.Done():
			return alerts
		case <-time.After(2 * time.Second):
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("reddit: new mentions found")
	}
	return alerts
}

func (m *Monitor) fetchSubreddit(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow, subreddit string) []mentionAlert {
	posts, err := m.sources().SubredditNew(ctx, subreddit, sinceFrom(ctx))
	if m.handleRedditErr(err, subreddit) {
		return nil
	}
	return m.insertPosts(ctx, wsID, kw, posts)
}

// handleRedditErr logs a Reddit failure and sets the backoff on 429/403.
// Returns true when err is non-nil.
func (m *Monitor) handleRedditErr(err error, target string) bool {
	if err == nil {
		return false
	}
	var se *StatusError
	switch {
	case errors.As(err, &se) && se.Code == http.StatusTooManyRequests:
		m.setRedditBackoff(10 * time.Minute)
		m.logger.Warn().Str("subreddit", target).Dur("backoff", 10*time.Minute).Msg("reddit: rate limited, backing off 10m")
	case errors.As(err, &se) && se.Code == http.StatusUnauthorized:
		m.setRedditBackoff(30 * time.Minute)
		m.logger.Warn().Str("subreddit", target).Msg("reddit: OAuth credentials rejected (401), backing off 30m — check REDDIT_CLIENT_ID/REDDIT_CLIENT_SECRET")
	case errors.As(err, &se) && se.Code == http.StatusForbidden:
		// Reddit now blocks unauthenticated JSON API access entirely.
		// Back off for 30 minutes to avoid spamming 403s every tick.
		m.setRedditBackoff(30 * time.Minute)
		m.logger.Warn().Str("subreddit", target).Dur("backoff", 30*time.Minute).Msg("reddit: blocked (403), backing off 30m — configure authenticated crawl via Pinchtab or Reddit OAuth2")
	default:
		m.logger.Warn().Err(err).Str("subreddit", target).Msg("reddit: request failed")
	}
	return true
}
