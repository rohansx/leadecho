package onboarding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"leadecho/internal/netsafe"
)

// Page is one crawled page of the product site.
type Page struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"-"`
}

// PageFetcher returns the raw HTML of a URL.
type PageFetcher func(ctx context.Context, rawURL string) (string, error)

const (
	MaxCrawlPages   = 8
	maxPageBytes    = 1 << 20
	maxPageText     = 20000
	perPageTimeout  = 10 * time.Second
	crawlFanout     = 3
	maxRedirectHops = 5
)

// priorityPaths are the page kinds worth grounding replies on, best first.
var priorityPaths = []string{"pricing", "features", "product", "docs", "about", "faq", "how-it-works", "use-cases", "solutions", "integrations", "customers"}

// SafeFetcher fetches user-supplied URLs through the netsafe SSRF guard.
// Redirects are followed (bounded): every hop's dial is re-checked by netsafe.
func SafeFetcher(timeout time.Duration) PageFetcher {
	c := netsafe.Client(timeout)
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirectHops {
			return errors.New("too many redirects")
		}
		return nil
	}
	return PlainFetcher(c)
}

// PlainFetcher fetches with the given client (tests; trusted URLs).
func PlainFetcher(c *http.Client) PageFetcher {
	return func(ctx context.Context, rawURL string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LeadEcho/1.0)")
		resp, err := c.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
		return string(b), err
	}
}

// NormalizeProductURL adds a scheme and validates an http(s) URL.
func NormalizeProductURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if !isHTTPURL(s) {
		return "", errors.New("url must be an http(s) URL")
	}
	return s, nil
}

// Crawl fetches the start page plus up to maxPages-1 same-host priority pages
// (pricing, features, docs, about, ...). Only the start page failing is an error.
func Crawl(ctx context.Context, fetch PageFetcher, startURL string, maxPages int, progress func(string)) ([]Page, error) {
	if progress == nil {
		progress = func(string) {}
	}
	base, err := url.Parse(startURL)
	if err != nil {
		return nil, err
	}
	progress("Reading " + base.Host)
	home, links, err := fetchPage(ctx, fetch, base)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", startURL, err)
	}
	pages := []Page{home}
	targets := pickLinks(base, links, maxPages-1)

	results := make([]*Page, len(targets))
	sem := make(chan struct{}, crawlFanout)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			u, _ := url.Parse(t)
			mu.Lock()
			progress("Reading " + u.Path + " page")
			mu.Unlock()
			if p, _, err := fetchPage(ctx, fetch, u); err == nil && p.Text != "" {
				results[i] = &p
			}
		}(i, t)
	}
	wg.Wait()
	for _, p := range results {
		if p != nil {
			pages = append(pages, *p)
		}
	}
	return pages, nil
}

func fetchPage(ctx context.Context, fetch PageFetcher, u *url.URL) (Page, []string, error) {
	pctx, cancel := context.WithTimeout(ctx, perPageTimeout)
	defer cancel()
	body, err := fetch(pctx, u.String())
	if err != nil {
		return Page{}, nil, err
	}
	title, text, links := extractPage(body, u)
	return Page{URL: u.String(), Title: title, Text: capLen(text, maxPageText)}, links, nil
}

// pickLinks keeps same-host links whose path matches a priority page kind,
// ordered by that priority, deduped, at most n.
func pickLinks(base *url.URL, links []string, n int) []string {
	host := strings.TrimPrefix(base.Hostname(), "www.")
	best := map[int][]string{}
	seen := map[string]bool{strings.TrimRight(base.String(), "/"): true}
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil || strings.TrimPrefix(u.Hostname(), "www.") != host {
			continue
		}
		u.Fragment, u.RawQuery = "", ""
		key := strings.TrimRight(u.String(), "/")
		if seen[key] {
			continue
		}
		path := strings.ToLower(u.Path)
		for rank, kind := range priorityPaths {
			if strings.Contains(path, kind) {
				seen[key] = true
				best[rank] = append(best[rank], u.String())
				break
			}
		}
	}
	var out []string
	for rank := range priorityPaths {
		for _, l := range best[rank] {
			if len(out) == n {
				return out
			}
			out = append(out, l)
		}
	}
	return out
}

var skipTextTags = map[string]bool{
	"script": true, "style": true, "nav": true, "footer": true,
	"noscript": true, "svg": true, "head": true,
}

// extractPage returns the title, visible text and absolute http(s) links.
// Links are collected everywhere (pricing usually sits in the nav); text skips chrome.
func extractPage(body string, base *url.URL) (title, text string, links []string) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", "", nil
	}
	var sb strings.Builder
	desc := "" // meta/og description: the site's own one-line pitch, ahead of header chrome
	var walk func(n *html.Node, inSkip bool)
	walk = func(n *html.Node, inSkip bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil && title == "" {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				if desc == "" {
					desc = metaDescription(n.Attr)
				}
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" {
						if u, err := base.Parse(strings.TrimSpace(a.Val)); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
							links = append(links, u.String())
						}
					}
				}
			}
			inSkip = inSkip || skipTextTags[n.Data]
		}
		if n.Type == html.TextNode && !inSkip {
			if t := strings.TrimSpace(n.Data); t != "" {
				sb.WriteString(t)
				sb.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inSkip)
		}
	}
	walk(doc, false)
	text = strings.TrimSpace(sb.String())
	if desc != "" {
		text = desc + " " + text
	}
	return title, text, links
}

// metaDescription returns content of <meta name="description"> or og:description.
func metaDescription(attrs []html.Attribute) string {
	var key, content string
	for _, a := range attrs {
		switch a.Key {
		case "name", "property":
			key = strings.ToLower(a.Val)
		case "content":
			content = strings.TrimSpace(a.Val)
		}
	}
	if key == "description" || key == "og:description" {
		return content
	}
	return ""
}
