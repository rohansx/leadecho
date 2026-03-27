package browser

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// FetchPageText fetches a URL via plain HTTP and extracts visible text from the body.
// Used as a fallback when the Scrapling sidecar is unavailable.
func FetchPageText(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max
	if err != nil {
		return "", err
	}

	text := extractText(string(body))
	if len(text) > 10000 {
		text = text[:10000]
	}
	return text, nil
}

// extractText parses HTML and returns visible text, stripping script/style/nav/footer tags.
func extractText(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	var sb strings.Builder
	skipTags := map[string]bool{
		"script": true, "style": true, "nav": true, "footer": true,
		"noscript": true, "svg": true, "head": true,
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(sb.String())
}
