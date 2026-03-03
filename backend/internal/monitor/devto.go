package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	// Dev.to tag search works best with single lowercase words
	tag := url.QueryEscape(kw.Term)
	apiURL := fmt.Sprintf("https://dev.to/api/articles?tag=%s&per_page=25&state=fresh", tag)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("devto: failed to create request")
		return nil
	}
	req.Header.Set("User-Agent", "LeadEcho/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("devto: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		m.logger.Warn().Int("status", resp.StatusCode).Str("keyword", kw.Term).Msg("devto: non-200 response")
		return nil
	}

	var articles []devtoArticle
	if err := json.NewDecoder(resp.Body).Decode(&articles); err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("devto: failed to decode response")
		return nil
	}

	var alerts []mentionAlert
	for _, a := range articles {
		content := a.Title
		if a.Description != "" {
			content = a.Title + "\n\n" + a.Description
		}
		if content == "" {
			continue
		}

		if !filterContent(content, kw) {
			continue
		}

		articleURL := a.URL
		if a.CanonicalURL != "" {
			articleURL = a.CanonicalURL
		}

		var publishedAt pgtype.Timestamptz
		if t, err := time.Parse(time.RFC3339, a.PublishedAt); err == nil {
			publishedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          "devto",
			PlatformID:        "devto_" + strconv.Itoa(a.ID),
			Url:               articleURL,
			Title:             pgtextPtr(a.Title),
			Content:           content,
			AuthorUsername:     pgtextPtr(a.User.Username),
			AuthorProfileUrl:  pgtextPtr("https://dev.to/" + a.User.Username),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"tags": a.TagList}),
			EngagementMetrics: jsonBytes(map[string]any{"reactions": a.PositiveReactionsCount, "comments": a.CommentsCount}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: publishedAt,
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("devto: new mentions found")
	}
	return alerts
}
