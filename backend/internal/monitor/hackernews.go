package monitor

import (
	"context"
	"html"
	"regexp"
	"strings"

	"leadecho/internal/database"
)

// HN Algolia API response structures.
type hnSearchResult struct {
	Hits []hnHit `json:"hits"`
}

type hnHit struct {
	ObjectID    string  `json:"objectID"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Author      string  `json:"author"`
	Points      int     `json:"points"`
	NumComments int     `json:"num_comments"`
	StoryText   string  `json:"story_text"`
	CommentText string  `json:"comment_text"`
	CreatedAtI  int64   `json:"created_at_i"`
	StoryID     float64 `json:"story_id"`
}

func (m *Monitor) crawlHackerNews(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	posts, err := m.sources().SearchHackerNews(ctx, kw.Term, sinceFrom(ctx))
	if err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("hn: search failed")
		return nil
	}
	alerts := m.insertPosts(ctx, wsID, kw, posts)
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("hn: new mentions found")
	}
	return alerts
}

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// cleanHNText turns Algolia's HTML (<p>, <a>, <i>, &#x27;) into plain text so
// the inbox, the scorer and reply drafts see what a reader sees.
func cleanHNText(s string) string {
	s = strings.ReplaceAll(s, "<p>", "\n\n")
	s = htmlTagRE.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}
