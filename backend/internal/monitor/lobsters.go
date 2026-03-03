package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

type lobstersStory struct {
	ShortID      string   `json:"short_id"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Description  string   `json:"description"`
	CommentsURL  string   `json:"comments_url"`
	Score        int      `json:"score"`
	CommentCount int      `json:"comment_count"`
	Tags         []string `json:"tags"`
	CreatedAt    string   `json:"created_at"`
	Submitter    struct {
		Username string `json:"username"`
	} `json:"submitter_user"`
}

func (m *Monitor) crawlLobsters(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	// Lobsters has no search API — fetch newest and filter locally
	req, err := http.NewRequestWithContext(ctx, "GET", "https://lobste.rs/newest.json", nil)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("lobsters: failed to create request")
		return nil
	}
	req.Header.Set("User-Agent", "LeadEcho/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("lobsters: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		m.logger.Warn().Int("status", resp.StatusCode).Str("keyword", kw.Term).Msg("lobsters: non-200 response")
		return nil
	}

	var stories []lobstersStory
	if err := json.NewDecoder(resp.Body).Decode(&stories); err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("lobsters: failed to decode response")
		return nil
	}

	var alerts []mentionAlert
	for _, s := range stories {
		content := s.Title
		if s.Description != "" {
			content = s.Title + "\n\n" + s.Description
		}
		if content == "" {
			continue
		}

		if !filterContent(content, kw) {
			continue
		}

		storyURL := s.CommentsURL
		if storyURL == "" {
			storyURL = s.URL
		}

		var createdAt pgtype.Timestamptz
		if t, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
			createdAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          "lobsters",
			PlatformID:        "lobsters_" + s.ShortID,
			Url:               storyURL,
			Title:             pgtextPtr(s.Title),
			Content:           content,
			AuthorUsername:     pgtextPtr(s.Submitter.Username),
			AuthorProfileUrl:  pgtextPtr("https://lobste.rs/~" + s.Submitter.Username),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"tags": s.Tags, "external_url": s.URL}),
			EngagementMetrics: jsonBytes(map[string]any{"score": s.Score, "comments": s.CommentCount}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: createdAt,
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("lobsters: new mentions found")
	}
	return alerts
}
