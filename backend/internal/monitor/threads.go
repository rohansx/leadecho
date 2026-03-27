package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"leadecho/internal/browser"
	"leadecho/internal/database"
)

// ThreadComment represents a single comment in a thread context.
type ThreadComment struct {
	Author string `json:"author"`
	Text   string `json:"text"`
	Score  int    `json:"score,omitempty"`
}

// FetchThreadContext retrieves thread comments for a mention, using cache when available.
// Returns serialized thread context text suitable for LLM prompts, truncated to ~4000 chars.
func FetchThreadContext(ctx context.Context, q *database.Queries, scrapling *browser.ScraplingClient, mention database.Mention) (string, error) {
	// Check cache first
	existing, err := q.GetThreadByMention(ctx, mention.ID)
	if err == nil && existing.Content != nil {
		return threadContentToText(existing.Content), nil
	}

	// Fetch based on platform
	var comments []ThreadComment
	switch mention.Platform {
	case "reddit":
		comments, err = fetchRedditThread(ctx, mention.Url)
	case "hackernews":
		comments, err = fetchHNThread(ctx, mention.Url)
	default:
		// For other platforms, try scraping if available
		if scrapling != nil {
			comments, err = fetchViaScraping(ctx, scrapling, mention.Url)
		} else {
			return "", nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("fetch thread for %s: %w", mention.Platform, err)
	}
	if len(comments) == 0 {
		return "", nil
	}

	contentJSON, _ := json.Marshal(comments)

	// Cache the thread
	q.CreateThread(ctx, database.CreateThreadParams{
		MentionID: mention.ID,
		Platform:  mention.Platform,
		ThreadID:  mention.PlatformID,
		Content:   contentJSON,
	})

	return threadContentToText(contentJSON), nil
}

// fetchRedditThread fetches comments from a Reddit thread using the JSON API.
func fetchRedditThread(ctx context.Context, url string) ([]ThreadComment, error) {
	// Reddit JSON API: append .json to the URL
	jsonURL := strings.TrimSuffix(url, "/") + ".json"

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jsonURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	return parseRedditJSON(body), nil
}

// parseRedditJSON extracts comments from Reddit's JSON API response.
func parseRedditJSON(data []byte) []ThreadComment {
	// Reddit returns an array of 2 listings: [post_listing, comments_listing]
	var listings []struct {
		Data struct {
			Children []struct {
				Data struct {
					Author string `json:"author"`
					Body   string `json:"body"`
					Score  int    `json:"score"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &listings); err != nil || len(listings) < 2 {
		return nil
	}

	var comments []ThreadComment
	for _, child := range listings[1].Data.Children {
		if child.Data.Body == "" || child.Data.Author == "" {
			continue
		}
		text := child.Data.Body
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		comments = append(comments, ThreadComment{
			Author: child.Data.Author,
			Text:   text,
			Score:  child.Data.Score,
		})
		if len(comments) >= 10 {
			break
		}
	}
	return comments
}

// fetchHNThread fetches comments from a Hacker News thread using the Algolia API.
func fetchHNThread(ctx context.Context, url string) ([]ThreadComment, error) {
	// Extract HN item ID from URL
	itemID := extractHNItemID(url)
	if itemID == "" {
		return nil, fmt.Errorf("could not extract HN item ID from URL: %s", url)
	}

	apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/items/%s", itemID)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HN API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var item struct {
		Children []struct {
			Author string `json:"author"`
			Text   string `json:"text"`
		} `json:"children"`
	}
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, err
	}

	var comments []ThreadComment
	for _, c := range item.Children {
		if c.Text == "" {
			continue
		}
		text := c.Text
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		comments = append(comments, ThreadComment{
			Author: c.Author,
			Text:   text,
		})
		if len(comments) >= 10 {
			break
		}
	}
	return comments, nil
}

// extractHNItemID extracts the item ID from a Hacker News URL.
func extractHNItemID(url string) string {
	// https://news.ycombinator.com/item?id=12345
	idx := strings.Index(url, "id=")
	if idx == -1 {
		return ""
	}
	id := url[idx+3:]
	// Trim any trailing query params or fragments
	for i, c := range id {
		if c == '&' || c == '#' {
			return id[:i]
		}
	}
	return id
}

// fetchViaScraping uses the Scrapling sidecar to get page text for non-standard platforms.
func fetchViaScraping(ctx context.Context, scrapling *browser.ScraplingClient, url string) ([]ThreadComment, error) {
	if err := scrapling.Navigate(ctx, url); err != nil {
		return nil, err
	}
	text, err := scrapling.GetText(ctx)
	if err != nil {
		return nil, err
	}
	if len(text) > 4000 {
		text = text[:4000]
	}
	// Return as a single "comment" representing the page content
	return []ThreadComment{{Author: "page", Text: text}}, nil
}

// threadContentToText converts stored thread JSON into an LLM-friendly text format.
func threadContentToText(contentJSON []byte) string {
	var comments []ThreadComment
	if err := json.Unmarshal(contentJSON, &comments); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, c := range comments {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", c.Author, c.Text))
	}

	text := sb.String()
	if len(text) > 4000 {
		text = text[:4000]
	}
	return text
}
