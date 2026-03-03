package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	// Search recent stories and comments via Algolia API
	query := url.QueryEscape(kw.Term)
	apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/search_by_date?query=%s&tags=(story,comment)&hitsPerPage=25", query)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("hn: failed to create request")
		return nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("hn: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		m.logger.Warn().Int("status", resp.StatusCode).Str("keyword", kw.Term).Msg("hn: non-200 response")
		return nil
	}

	var result hnSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("hn: failed to decode response")
		return nil
	}

	var alerts []mentionAlert
	for _, hit := range result.Hits {
		content := hit.Title
		if hit.StoryText != "" {
			content = hit.Title + "\n\n" + hit.StoryText
		}
		if hit.CommentText != "" {
			content = hit.CommentText
		}
		if content == "" {
			continue
		}

		if !filterContent(content, kw) {
			continue
		}

		itemURL := fmt.Sprintf("https://news.ycombinator.com/item?id=%s", hit.ObjectID)

		createdAt := time.Unix(hit.CreatedAtI, 0)

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          string(database.PlatformTypeHackernews),
			PlatformID:        "hn_" + hit.ObjectID,
			Url:               itemURL,
			Title:             pgtextPtr(hit.Title),
			Content:           content,
			AuthorUsername:     pgtextPtr(hit.Author),
			AuthorProfileUrl:  pgtextPtr("https://news.ycombinator.com/user?id=" + hit.Author),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"points": hit.Points, "external_url": hit.URL}),
			EngagementMetrics: jsonBytes(map[string]any{"points": hit.Points, "num_comments": hit.NumComments}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("hn: new mentions found")
	}
	return alerts
}
