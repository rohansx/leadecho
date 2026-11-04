package monitor

import (
	"context"

	"leadecho/internal/database"
)

type devtoArticle struct {
	ID                     int      `json:"id"`
	Title                  string   `json:"title"`
	Description            string   `json:"description"`
	URL                    string   `json:"url"`
	CanonicalURL           string   `json:"canonical_url"`
	PublishedAt            string   `json:"published_at"`
	PositiveReactionsCount int      `json:"positive_reactions_count"`
	CommentsCount          int      `json:"comments_count"`
	TagList                []string `json:"tag_list"`
	User                   struct {
		Username string `json:"username"`
	} `json:"user"`
}

func (m *Monitor) crawlDevTo(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	posts, err := m.sources().SearchDevTo(ctx, kw.Term, sinceFrom(ctx))
	if err != nil {
		m.logger.Warn().Err(err).Str("keyword", kw.Term).Msg("devto: search failed")
		return nil
	}
	alerts := m.insertPosts(ctx, wsID, kw, posts)
	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("devto: new mentions found")
	}
	return alerts
}
