package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

type ihRSS struct {
	Channel struct {
		Items []ihRSSItem `xml:"item"`
	} `xml:"channel"`
}

type ihRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Creator     string `xml:"creator"`
}

func (m *Monitor) crawlIndieHackers(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	// IndieHackers retired their RSS feed (feed.xml now redirects to the homepage).
	// Instead of scraping the SPA (which needs a full browser), we search for
	// the keyword on IndieHackers via Google News, which indexes IH posts.
	// This gives us IH content without needing a sidecar.
	query := url.QueryEscape("site:indiehackers.com " + kw.Term)
	apiURL := fmt.Sprintf("https://news.google.com/rss/search?q=%s&hl=en-US&gl=US&ceid=US:en", query)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("indiehackers: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		m.logger.Warn().Int("status", resp.StatusCode).Str("keyword", kw.Term).Msg("indiehackers: non-200 response")
		return nil
	}

	var feed ihRSS
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("indiehackers: failed to decode RSS")
		return nil
	}

	var alerts []mentionAlert
	for _, item := range feed.Channel.Items {
		content := stripHTML(item.Description)
		if content == "" {
			content = item.Title
		}
		if content == "" || len(content) < 20 {
			continue
		}

		if !filterContent(content, kw) {
			continue
		}

		h := sha256.Sum256([]byte(item.Link))
		platformID := "ih_" + hex.EncodeToString(h[:8])

		var pubDate pgtype.Timestamptz
		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			pubDate = pgtype.Timestamptz{Time: t, Valid: true}
		} else if t, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
			pubDate = pgtype.Timestamptz{Time: t, Valid: true}
		}

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          "indiehackers",
			PlatformID:        platformID,
			Url:               item.Link,
			Title:             pgtextPtr(item.Title),
			Content:           content,
			AuthorUsername:     pgtextPtr("indiehacker"),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"source": "google_news_search"}),
			EngagementMetrics: jsonBytes(map[string]any{}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: pubDate,
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("indiehackers: new mentions found")
	}
	return alerts
}

// stripHTML removes HTML tags from a string (simple approach).
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
