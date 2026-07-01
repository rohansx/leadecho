package monitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

const (
	exaNumResults   = 10
	exaLookbackDays = 14       // only surface pages published in the last N days
	exaMaxContent   = 4000     // cap stored content (runes) — Exa returns full page text
	exaMaxRespBytes = 10 << 20 // 10MB cap on the raw Exa response body
)

// exaSearchURL is the Exa search endpoint. It is a var (not const) so tests can
// point the crawler at a local mock server.
var exaSearchURL = "https://api.exa.ai/search"

// exaSearchRequest is the POST body for the Exa /search endpoint.
type exaSearchRequest struct {
	Query              string          `json:"query"`
	Type               string          `json:"type"`
	NumResults         int             `json:"numResults"`
	StartPublishedDate string          `json:"startPublishedDate,omitempty"`
	Contents           exaContentsOpts `json:"contents"`
}

type exaContentsOpts struct {
	Text       bool `json:"text"`
	Highlights bool `json:"highlights"`
}

type exaSearchResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Author        string   `json:"author"`
	PublishedDate string   `json:"publishedDate"`
	Text          string   `json:"text"`
	Highlights    []string `json:"highlights"`
}

// exaHTTPClient bounds Exa calls; search + content extraction can be slow.
var exaHTTPClient = &http.Client{Timeout: 30 * time.Second}

// crawlExa queries Exa's web-wide semantic search for a keyword and turns
// fresh matching pages into mentions. Unlike the platform-specific crawlers,
// this discovers content across the entire web (blogs, forums, news, comparison
// sites) rather than a single fixed site. Requires EXA_API_KEY; the dispatcher
// skips this source when the key is unset.
func (m *Monitor) crawlExa(ctx context.Context, wsID string, kw database.ListActiveKeywordsRow) []mentionAlert {
	results, err := m.fetchExa(ctx, kw.Term)
	if err != nil {
		m.logger.Error().Err(err).Str("keyword", kw.Term).Msg("exa: fetch failed")
		return nil
	}

	var alerts []mentionAlert
	for _, hit := range results {
		link := exaResultLink(hit)
		if link == "" || !isHTTPURL(link) {
			continue
		}

		content := exaResultContent(hit)
		if content == "" {
			continue
		}
		if !filterContent(content, kw) {
			m.logger.Debug().Str("keyword", kw.Term).Str("url", link).Msg("exa: content filtered out")
			continue
		}

		// Dedup on Exa's canonical id rather than the resolved URL, since the
		// same page can resolve to multiple URL variants (www vs. bare domain,
		// trailing slash, etc.) but shares one canonical id.
		dedupKey := hit.ID
		if dedupKey == "" {
			dedupKey = link
		}
		h := sha256.Sum256([]byte(dedupKey))
		platformID := "exa_" + hex.EncodeToString(h[:8])

		alert := m.insertMention(ctx, database.CreateMentionParams{
			WorkspaceID:       wsID,
			KeywordID:         pgUUID(kw.ID),
			Platform:          string(database.PlatformTypeExa),
			PlatformID:        platformID,
			Url:               link,
			Title:             pgtextPtr(hit.Title),
			Content:           content,
			AuthorUsername:    pgtextPtr(hit.Author),
			Status:            database.MentionStatusNew,
			PlatformMetadata:  jsonBytes(map[string]any{"source": "exa", "exa_id": hit.ID}),
			EngagementMetrics: jsonBytes(map[string]any{}),
			KeywordMatches:    []string{kw.Term},
			PlatformCreatedAt: exaResultPublished(hit),
		}, kw.Term)

		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info().Int("count", len(alerts)).Str("keyword", kw.Term).Msg("exa: new mentions found")
	}
	return alerts
}

// fetchExa performs the Exa /search call for a single term and returns the raw
// results. It is the only part of the source that touches the network.
func (m *Monitor) fetchExa(ctx context.Context, term string) ([]exaResult, error) {
	reqBody := exaSearchRequest{
		Query:              term,
		Type:               "auto",
		NumResults:         exaNumResults,
		StartPublishedDate: time.Now().AddDate(0, 0, -exaLookbackDays).Format(time.RFC3339),
		Contents:           exaContentsOpts{Text: true, Highlights: true},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", exaSearchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.exaAPIKey)

	resp, err := exaHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exa: non-200 response (status %d)", resp.StatusCode)
	}

	var result exaSearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, exaMaxRespBytes)).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		m.logger.Info().Str("keyword", term).Msg("exa: no results found")
	}
	return result.Results, nil
}

// exaResultLink returns the canonical URL for a result (Exa's id is the
// canonical URL; url is the resolved one). Empty when neither is present.
func exaResultLink(hit exaResult) string {
	if hit.URL != "" {
		return hit.URL
	}
	return hit.ID
}

// exaResultContent builds the mention body from a result: the title plus the
// page text (falling back to highlight snippets), capped at exaMaxContent runes.
func exaResultContent(hit exaResult) string {
	body := strings.TrimSpace(hit.Text)
	if body == "" {
		body = strings.TrimSpace(strings.Join(hit.Highlights, " "))
	}
	return truncateRunes(strings.TrimSpace(hit.Title+"\n\n"+body), exaMaxContent)
}

// exaResultPublished parses the result's publish date. Returns a NULL
// timestamp (rather than fabricating "now") when the date is absent or
// malformed — Exa omits it for some pages, and a fake "now" would misrepresent
// how fresh the page actually is for chronological sorting.
func exaResultPublished(hit exaResult) pgtype.Timestamptz {
	if hit.PublishedDate != "" {
		if t, err := time.Parse(time.RFC3339, hit.PublishedDate); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	return pgtype.Timestamptz{}
}

// isHTTPURL reports whether s is an absolute http(s) URL. Exa results should
// always be http(s), but this rejects schemes like javascript: or data: before
// they're stored and later rendered as a clickable link in the dashboard.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// truncateRunes caps s at max runes (not bytes) so multibyte content is never
// split mid-character.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
