package competitors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"leadecho/internal/onboarding"
)

const (
	// MinPageText: a page with less visible text than this (runes) did not
	// really load (rendered in the browser, a bot wall, an error page). That is
	// a failed check, never "unchanged".
	MinPageText = 200

	maxPageRunes   = 20000
	maxDiffRunes   = 4000
	maxGitHubRepos = 3

	// feedBaseline is the last_hash of a source read as a feed or as posts:
	// it only records that a first check happened.
	feedBaseline = "feed"
)

var (
	// ErrEmptyPage: the page came back with almost no text.
	ErrEmptyPage = errors.New("page has no readable text")
	// ErrLocked: the source needs paid data this workspace does not have.
	ErrLocked = errors.New("paid data not available")
)

// Fetched is the outcome of checking one source.
type Fetched struct {
	Items []Item
	Hash  string // new last_hash
	Text  string // new last_text (page kinds)
}

var skipText = map[string]bool{"script": true, "style": true, "nav": true, "footer": true, "noscript": true, "svg": true, "head": true}

// PageText returns the visible text of an HTML page, one text run per line,
// without navigation, footer and scripts.
func PageText(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	var lines []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipText[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				lines = append(lines, t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.Join(lines, "\n")
}

var noiseLines = []*regexp.Regexp{
	regexp.MustCompile(`^[\d\s.,:%/+\-]+$`), // counters, numeric dates
	regexp.MustCompile(`(?i)^((mon|tue|wed|thu|fri|sat|sun)[a-z]*,?\s+)?(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+\d{1,2}(st|nd|rd|th)?,?(\s+\d{4})?$`),
	regexp.MustCompile(`(?i)^\d{1,2}(st|nd|rd|th)?\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?,?(\s+\d{4})?$`),
	regexp.MustCompile(`(?i)^(©|\(c\)|copyright)`),
	regexp.MustCompile(`(?i)^(\d+|an?)\s+(second|minute|hour|day|week|month|year)s?\s+ago$`),
}

// NormalizeText drops what changes on a page without anything having
// changed: spacing, bare dates and counters, the copyright line.
//
// ponytail: line rules only. Rotating testimonials or A/B-tested headlines
// will still read as changes; the model step drops those as "not a move".
func NormalizeText(s string) string {
	var out []string
lines:
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		for _, re := range noiseLines {
			if re.MatchString(line) {
				continue lines
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// Hash is the fingerprint stored as a source's last_hash.
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// lineDiff returns the lines only in old and the lines only in new, in order.
func lineDiff(old, cur string) (removed, added []string) {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	o, c := in(old), in(cur)
	for _, l := range strings.Split(old, "\n") {
		if !c[l] {
			removed = append(removed, l)
		}
	}
	for _, l := range strings.Split(cur, "\n") {
		if !o[l] {
			added = append(added, l)
		}
	}
	return removed, added
}

var changeTitles = map[string]string{
	KindLanding:   "Homepage changed",
	KindPricing:   "Pricing page changed",
	KindChangelog: "Changelog updated",
	KindBlog:      "Blog updated",
}

// FetchURL checks one web source (every kind except x and linkedin).
// lastHash "" means this is the first check: it records a baseline and
// returns no items, so adding a competitor does not replay its history.
// (For feeds the caller stores the returned items as already seen.)
func FetchURL(ctx context.Context, fetch onboarding.PageFetcher, kind, rawURL, lastHash, lastText string) (Fetched, error) {
	if kind == KindGitHub {
		return fetchGitHub(ctx, fetch, rawURL)
	}
	body, err := fetch(ctx, rawURL)
	if err != nil {
		return Fetched{}, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	if kind == KindBlog || kind == KindChangelog {
		if items, err := ParseFeed([]byte(body)); err == nil {
			return Fetched{Items: items, Hash: feedBaseline}, nil
		}
		if feedURL := FeedLink(rawURL, body); feedURL != "" {
			if fb, err := fetch(ctx, feedURL); err == nil {
				if items, err := ParseFeed([]byte(fb)); err == nil {
					return Fetched{Items: items, Hash: feedBaseline}, nil
				}
			}
		}
	}
	return pageChange(kind, rawURL, body, lastHash, lastText)
}

func pageChange(kind, rawURL, body, lastHash, lastText string) (Fetched, error) {
	text := clip(NormalizeText(PageText(body)), maxPageRunes)
	if len([]rune(text)) < MinPageText {
		return Fetched{}, ErrEmptyPage
	}
	out := Fetched{Hash: Hash(text), Text: text}
	if lastHash == "" || lastHash == feedBaseline || lastHash == out.Hash {
		return out, nil
	}
	removed, added := lineDiff(lastText, text)
	if len(removed)+len(added) == 0 { // only the order changed
		return out, nil
	}
	var b strings.Builder
	if len(removed) > 0 {
		b.WriteString("REMOVED:\n" + clip(strings.Join(removed, "\n"), maxDiffRunes) + "\n\n")
	}
	if len(added) > 0 {
		b.WriteString("ADDED:\n" + clip(strings.Join(added, "\n"), maxDiffRunes))
	}
	out.Items = []Item{{Key: out.Hash, Title: changeTitles[kind], Body: strings.TrimSpace(b.String()), URL: rawURL, At: time.Now().UTC()}}
	return out, nil
}

// fetchGitHub reads the release feeds of up to maxGitHubRepos repositories
// linked from an organisation's page. A repository with no releases, or one
// that cannot be read, is skipped.
func fetchGitHub(ctx context.Context, fetch onboarding.PageFetcher, orgURL string) (Fetched, error) {
	base, err := url.Parse(orgURL)
	if err != nil {
		return Fetched{}, fmt.Errorf("parse %s: %w", orgURL, err)
	}
	org := strings.ToLower(strings.Trim(base.Path, "/"))
	body, err := fetch(ctx, orgURL)
	if err != nil {
		return Fetched{}, fmt.Errorf("fetch %s: %w", orgURL, err)
	}
	var repos []string
	seen := map[string]bool{}
	for _, l := range onboarding.PageLinks(body, base) {
		u, err := url.Parse(l)
		if err != nil || u.Hostname() != "github.com" {
			continue
		}
		segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		if len(segs) != 2 || strings.ToLower(segs[0]) != org || !slugRe.MatchString(segs[1]) || seen[strings.ToLower(segs[1])] {
			continue
		}
		seen[strings.ToLower(segs[1])] = true
		if repos = append(repos, segs[1]); len(repos) == maxGitHubRepos {
			break
		}
	}
	out := Fetched{Hash: feedBaseline}
	for _, repo := range repos {
		fb, err := fetch(ctx, "https://github.com/"+org+"/"+repo+"/releases.atom")
		if err != nil {
			continue
		}
		items, err := ParseFeed([]byte(fb))
		if err != nil {
			continue
		}
		for _, it := range items {
			it.Title = strings.TrimSpace(repo + " " + it.Title)
			out.Items = append(out.Items, it)
		}
	}
	return out, nil
}
