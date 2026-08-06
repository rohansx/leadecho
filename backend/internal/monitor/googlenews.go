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

// Google News exposes a keyword search as an RSS feed with no API key and no
// browser sidecar, which makes it the cheapest way to watch news and blog
// coverage of a term.
//
// Licensing caveat: the feed ships a copyright notice limiting it to "a
// personal feed reader for personal, non-commercial use". This crawler is
// therefore gated behind GOOGLE_NEWS_ENABLED (default false) so it never runs
// in a distributed build unless an operator opts in. For commercial use prefer
// the Exa source, which is licensed for programmatic access.
const googleNewsFeedURL = "https://news.google.com/rss/search"

type gnewsRSS struct {
	Channel struct {
		Items []gnewsItem `xml:"item"`
	} `xml:"channel"`
}

type gnewsItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Source      string `xml:"source"`
}

func (m *Monitor) crawlGoogleNews(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	term := strings.TrimSpace(kw.Term)
	if term == "" {
		return nil
	}

	// Quote the term so Google matches the phrase rather than loose words.
	q := url.Values{}
	q.Set("q", `"`+term+`"`)
	q.Set("hl", "en-US")
	q.Set("gl", "US")
	q.Set("ceid", "US:en")
	endpoint := googleNewsFeedURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", term).Msg("googlenews: failed to create request")
		return nil
	}
	req.Header.Set("User-Agent", "LeadEcho/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", term).Msg("googlenews: request failed")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		m.logger.Warn().Int("status", resp.StatusCode).Str("keyword", term).Msg("googlenews: non-200 response")
		return nil
	}

	var feed gnewsRSS
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		m.logger.Error().Err(err).Str("keyword", term).Msg("googlenews: failed to decode feed")
		return nil
	}

	var alerts []mentionAlert
	for _, it := range feed.Channel.Items {
		// Google appends " - Publisher" to every headline; the publisher is
		// already available separately, so trim it back off the title.
		title := trimGoogleNewsSuffix(it.Title, it.Source)
		if title == "" {
			continue
		}

		// description is an HTML anchor wrapping the headline, which adds no
		// information beyond the title. Strip tags and drop it if it collapses
		// to a repeat of the title.
		body := strings.TrimSpace(stripHTML(it.Description))
		content := title
		if body != "" && !strings.EqualFold(body, title) {
			content = title + "\n\n" + body
		}

		// The RSS feed is a search result, but Google matches loosely (stemming,
		// synonyms). Apply the keyword locally so the mention actually contains
		// the term, the same way the Lobsters firehose is filtered.
		if !strings.Contains(strings.ToLower(content), strings.ToLower(term)) {
			continue
		}
		if !filterContent(content, kw) {
			continue
		}

		// guid is a long opaque string; hash it so platform_id stays bounded.
		ident := it.GUID
		if ident == "" {
			ident = it.Link
		}
		sum := sha256.Sum256([]byte(ident))
		platformID := "googlenews_" + hex.EncodeToString(sum[:])[:32]

		var createdAt pgtype.Timestamptz
		if t, err := time.Parse(time.RFC1123, it.PubDate); err == nil {
			createdAt = pgtype.Timestamptz{Time: t, Valid: true}
		} else if t, err := time.Parse(time.RFC1123Z, it.PubDate); err == nil {
			createdAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          string(database.PlatformTypeGooglenews),
			PlatformID:        platformID,
			Url:               it.Link,
			Title:             pgtextPtr(title),
			Content:           content,
			AuthorUsername:    pgtextPtr(it.Source),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"publisher": it.Source}),
			EngagementMetrics: jsonBytes(map[string]any{}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: createdAt,
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", term).Msg("googlenews: new mentions found")
	}
	return alerts
}

// trimGoogleNewsSuffix removes the " - Publisher" that Google appends to every
// headline, so the stored title matches what the outlet actually published.
func trimGoogleNewsSuffix(title, source string) string {
	title = strings.TrimSpace(title)
	if source == "" {
		return title
	}
	suffix := " - " + strings.TrimSpace(source)
	return strings.TrimSpace(strings.TrimSuffix(title, suffix))
}

var _ = fmt.Sprintf
