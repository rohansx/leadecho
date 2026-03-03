package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
func (m *Monitor) crawlReddit(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	if len(kw.Subreddits) == 0 {
		return nil // no subreddits configured — skip
	}

	var alerts []mentionAlert
	for _, sub := range kw.Subreddits {
		results := m.fetchSubreddit(ctx, wsID, kw, sub)
		alerts = append(alerts, results...)

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
	url := fmt.Sprintf("https://www.reddit.com/r/%s/new.json?limit=25", subreddit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		m.logger.Error().Err(err).Str("subreddit", subreddit).Msg("reddit: failed to create request")
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("subreddit", subreddit).Msg("reddit: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		m.redditBackoff = time.Now().Add(10 * time.Minute)
		m.logger.Warn().Str("subreddit", subreddit).Time("retry_after", m.redditBackoff).Msg("reddit: rate limited, backing off 10m")
		return nil
	}

	if resp.StatusCode != 200 {
		m.logger.Warn().Int("status", resp.StatusCode).Str("subreddit", subreddit).Msg("reddit: non-200 response")
		return nil
	}

	var listing redditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		m.logger.Error().Err(err).Str("subreddit", subreddit).Msg("reddit: failed to decode response")
		return nil
	}

	var alerts []mentionAlert
	for _, child := range listing.Data.Children {
		post := child.Data
		if post.Author == "[deleted]" || post.Author == "AutoModerator" {
			continue
		}

		content := post.Title
		if post.Selftext != "" {
			content = post.Title + "\n\n" + post.Selftext
		}

		// Filter: keyword must appear in the post content
		if !filterContent(content, kw) {
			continue
		}

		createdAt := time.Unix(int64(post.CreatedUTC), 0)

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          string(database.PlatformTypeReddit),
			PlatformID:        "reddit_" + post.ID,
			Url:               "https://reddit.com" + post.Permalink,
			Title:             pgtextPtr(post.Title),
			Content:           content,
			AuthorUsername:     pgtextPtr(post.Author),
			AuthorProfileUrl:  pgtextPtr("https://reddit.com/u/" + post.Author),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"subreddit": post.Subreddit, "score": post.Score}),
			EngagementMetrics: jsonBytes(map[string]any{"score": post.Score, "num_comments": post.NumComments}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	return alerts
}
