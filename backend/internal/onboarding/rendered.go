package onboarding

import (
	"context"
	"encoding/json"
	"strings"

	"leadecho/internal/treg"
)

// MinPageText is the least text a product page must yield to be worth
// analysing. A site that builds its page in the browser answers a plain fetch
// with little more than a title.
const MinPageText = 200

// renderedPageTool reads one URL with a real browser when a plain fetch finds
// nothing ($0.00025 a page; a blocked or unreachable page is free).
const renderedPageTool = "crawl4ai.web.scrape"

// PageReader returns a page's text as a browser shows it.
type PageReader func(ctx context.Context, workspaceID, pageURL string) (string, error)

// TregPageReader reads pages through treg. Nil when treg is off.
func TregPageReader(c *treg.Client) PageReader {
	if !c.Enabled() {
		return nil
	}
	return func(ctx context.Context, workspaceID, pageURL string) (string, error) {
		resp, err := c.Call(ctx, treg.Request{WorkspaceID: workspaceID, Tool: renderedPageTool,
			Body: map[string]string{"url": pageURL, "format": "md"}})
		if err != nil {
			return "", err
		}
		return ParseRenderedPage(resp.Body), nil
	}
}

// ParseRenderedPage takes the Markdown out of a page-scrape answer, which
// arrives either bare or wrapped in "output". "" = nothing readable.
func ParseRenderedPage(body []byte) string {
	var r struct {
		Markdown string `json:"markdown"`
		Output   struct {
			Markdown string `json:"markdown"`
		} `json:"output"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	text := r.Markdown
	if text == "" {
		text = r.Output.Markdown
	}
	return capBytes(strings.TrimSpace(text), maxPageText)
}
