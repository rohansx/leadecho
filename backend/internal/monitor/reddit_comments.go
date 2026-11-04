package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// Reddit comment search: keyword hits inside comments become mentions.
// Uses the free public /r/<sub>/comments.json stream (newest ~100 comments)
// filtered locally, same approach and backoff as the post crawler.
// ponytail: free path only; treg crawl4ai.reddit.post.comments costs one call
// per post, add it when Reddit's public JSON is blocked for good.

const redditBaseURL = "https://www.reddit.com"

// SetRedditCommentsEnabled toggles comment search (REDDIT_COMMENTS_ENABLED).
func (m *Monitor) SetRedditCommentsEnabled(v bool) { m.redditCommentsEnabled = v }

type redditComment struct {
	ID         string  `json:"id"`
	Body       string  `json:"body"`
	Author     string  `json:"author"`
	Subreddit  string  `json:"subreddit"`
	Permalink  string  `json:"permalink"`
	LinkTitle  string  `json:"link_title"`
	LinkID     string  `json:"link_id"`
	Score      int     `json:"score"`
	CreatedUTC float64 `json:"created_utc"`
}

var subredditRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,50}$`)

func validSubreddit(s string) bool { return subredditRe.MatchString(s) }

func (m *Monitor) crawlRedditComments(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	var alerts []mentionAlert
	for i, sub := range kw.Subreddits {
		if m.redditBackedOff() || !validSubreddit(sub) {
			continue
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return alerts
			case <-time.After(2 * time.Second):
			}
		}
		comments, err := m.fetchSubredditComments(ctx, redditBaseURL, sub)
		if err != nil {
			m.logger.Warn().Err(err).Str("subreddit", sub).Msg("reddit-comments: fetch failed")
			continue
		}
		for _, c := range comments {
			if !filterContent(c.Body, kw) {
				continue
			}
			if a := m.insertMention(ctx, redditCommentParams(wsID, kw, c), kw.Term); a != nil {
				alerts = append(alerts, *a)
			}
		}
	}
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("reddit-comments: new mentions found")
	}
	return alerts
}

type redditCommentListing struct {
	Data struct {
		Children []struct {
			Data redditComment `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (m *Monitor) fetchSubredditComments(ctx context.Context, base, sub string) ([]redditComment, error) {
	var listing redditCommentListing
	var err error
	if o := m.sources().RedditOAuth; o != nil {
		err = o.getJSON(ctx, m.sources().Client, fmt.Sprintf("/r/%s/comments?limit=100", sub), &listing)
	} else {
		err = fetchPublicJSON(ctx, fmt.Sprintf("%s/r/%s/comments.json?limit=100", base, sub), &listing)
	}
	var se *StatusError
	switch {
	case err == nil:
	case errors.As(err, &se) && se.Code == http.StatusTooManyRequests:
		m.setRedditBackoff(10 * time.Minute)
		return nil, fmt.Errorf("rate limited")
	case errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusUnauthorized):
		m.setRedditBackoff(30 * time.Minute) // 401 = bad REDDIT_CLIENT_ID/SECRET
		return nil, fmt.Errorf("blocked (%d)", se.Code)
	default:
		return nil, err
	}
	var out []redditComment
	for _, ch := range listing.Data.Children {
		c := ch.Data
		if c.ID == "" || c.Author == "[deleted]" || c.Author == "AutoModerator" || c.Body == "[deleted]" || c.Body == "[removed]" {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func redditCommentParams(wsID string, kw database.ListActiveKeywordsRow, c redditComment) database.CreateMentionParams {
	return database.CreateMentionParams{
		WorkspaceID:       wsID,
		KeywordID:         pgUUID(kw.ID),
		Platform:          string(database.PlatformTypeReddit),
		PlatformID:        "reddit_comment_" + c.ID,
		Url:               "https://reddit.com" + c.Permalink,
		Title:             pgtextPtr(c.LinkTitle),
		Content:           c.Body,
		AuthorUsername:    pgtextPtr(c.Author),
		AuthorProfileUrl:  pgtextPtr("https://reddit.com/u/" + c.Author),
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(map[string]any{"subreddit": c.Subreddit, "kind": "comment", "link_id": c.LinkID, "score": c.Score}),
		EngagementMetrics: jsonBytes(map[string]any{"score": c.Score}),
		KeywordMatches:    []string{kw.Term},
		PlatformCreatedAt: pgtype.Timestamptz{Time: time.Unix(int64(c.CreatedUTC), 0), Valid: true},
	}
}

func fetchPublicJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSourceBody)).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}
