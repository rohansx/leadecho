package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

type lobstersStory struct {
	ShortID          string      `json:"short_id"`
	Title            string      `json:"title"`
	URL              string      `json:"url"`
	Description      string      `json:"description"`
	DescriptionPlain string      `json:"description_plain"`
	CommentsURL      string      `json:"comments_url"`
	Score        int             `json:"score"`
	CommentCount int             `json:"comment_count"`
	Tags         []string        `json:"tags"`
	CreatedAt    string          `json:"created_at"`
	Submitter    lobstersUsername `json:"submitter_user"`
}

// lobstersUsername accepts both shapes Lobsters has served for submitter_user:
// the current bare string ("PuercoPop") and the older {"username": "..."} object.
// Decoding the wrong one used to fail the whole page, silently yielding zero
// mentions for every keyword.
type lobstersUsername struct {
	Username string
}

func (u *lobstersUsername) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		u.Username = s
		return nil
	}
	var obj struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	u.Username = obj.Username
	return nil
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
	term := strings.ToLower(strings.TrimSpace(kw.Term))
	for _, s := range stories {
		// Prefer the plain-text body; `description` is raw HTML and leaks tags
		// into the mention content and the inbox preview.
		body := s.DescriptionPlain
		if body == "" {
			body = stripHTML(s.Description)
		}
		content := s.Title
		if body != "" {
			content = s.Title + "\n\n" + body
		}
		if content == "" {
			continue
		}

		// Lobsters has no search endpoint — this is the /newest firehose, so the
		// keyword has to be applied here. filterContent's default "contains" arm
		// is a pass-through that assumes the platform already searched, which
		// would admit every new post regardless of the keyword.
		if term != "" && !strings.Contains(strings.ToLower(content), term) {
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
