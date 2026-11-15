// Package competitors watches what a workspace's competitors publish and
// turns it into moves: a launch, a pricing change, a positioning change or
// company news, each with what it means for the customer and what to do.
//
// Nothing here is a mention. Competitor content never reaches lead scoring.
package competitors

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"leadecho/internal/onboarding"
)

// Source kinds: where a competitor publishes.
const (
	KindX         = "x"
	KindLinkedIn  = "linkedin"
	KindLanding   = "page_landing"
	KindPricing   = "page_pricing"
	KindChangelog = "changelog"
	KindBlog      = "blog"
	KindGitHub    = "github"
)

// Kinds lists every valid source kind.
var Kinds = []string{KindX, KindLinkedIn, KindLanding, KindPricing, KindChangelog, KindBlog, KindGitHub}

// ValidKind reports whether k is a source kind.
func ValidKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// Paid reports the kinds that need paid data (treg).
func Paid(kind string) bool { return kind == KindX || kind == KindLinkedIn }

// Source is one place a competitor publishes.
type Source struct{ Kind, URL string }

var (
	xHandleRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	slugRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

	// First path segments on x.com / github.com that are the site's own pages.
	xReserved  = set("intent", "share", "home", "i", "search", "hashtag", "privacy", "tos", "login", "explore", "settings", "compose")
	ghReserved = set("features", "pricing", "login", "join", "about", "sponsors", "marketplace", "orgs", "topics", "apps",
		"site", "enterprise", "readme", "security", "contact", "customer-stories", "collections", "trending", "explore",
		"solutions", "resources", "team", "open-source", "settings", "notifications", "pulls", "issues")

	changelogSegs = set("changelog", "changelogs", "whats-new", "what-s-new", "release-notes", "releases", "updates", "product-updates")
	blogSegs      = set("blog", "news")
	pricingSegs   = set("pricing", "plans", "prices")
)

func set(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

// ExtractSources reads the places a company publishes off its own homepage.
// Nobody confirms the result, so it prefers missing a source to watching the
// wrong one: a social or GitHub account counts only when the page links to
// exactly one.
func ExtractSources(pageURL, body string) []Source {
	base, err := url.Parse(pageURL)
	if err != nil || base.Host == "" {
		return nil
	}
	site := strings.TrimPrefix(strings.ToLower(base.Hostname()), "www.")
	out := []Source{{KindLanding, pageURL}}

	accounts := map[string]map[string]bool{KindX: {}, KindLinkedIn: {}, KindGitHub: {}}
	pages := map[string]string{} // kind -> shortest matching URL
	for _, l := range onboarding.PageLinks(body, base) {
		u, err := url.Parse(l)
		if err != nil {
			continue
		}
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		segs := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		switch {
		case host == "x.com" || host == "twitter.com":
			if len(segs) >= 1 && xHandleRe.MatchString(segs[0]) && !xReserved[strings.ToLower(segs[0])] {
				accounts[KindX]["https://x.com/"+strings.ToLower(segs[0])] = true
			}
		case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
			if len(segs) >= 2 && segs[0] == "company" && slugRe.MatchString(segs[1]) {
				accounts[KindLinkedIn]["https://www.linkedin.com/company/"+strings.ToLower(segs[1])] = true
			}
		case host == "github.com":
			if len(segs) >= 1 && slugRe.MatchString(segs[0]) && !ghReserved[strings.ToLower(segs[0])] {
				accounts[KindGitHub]["https://github.com/"+strings.ToLower(segs[0])] = true
			}
		case host == site || strings.HasSuffix(host, "."+site):
			if kind := pageKind(host, site, segs); kind != "" {
				u.Fragment, u.RawQuery = "", ""
				if cur, ok := pages[kind]; !ok || len(u.String()) < len(cur) {
					pages[kind] = u.String()
				}
			}
		}
	}
	for _, kind := range []string{KindPricing, KindChangelog, KindBlog} {
		if u, ok := pages[kind]; ok {
			out = append(out, Source{kind, u})
		}
	}
	for _, kind := range []string{KindX, KindLinkedIn, KindGitHub} {
		if len(accounts[kind]) == 1 {
			for u := range accounts[kind] {
				out = append(out, Source{kind, u})
			}
		}
	}
	return out
}

// pageKind classifies a same-site link by its subdomain label, its first
// path segment, or (for short paths like /docs/changelog) its last one. A
// post such as /blog/why-our-pricing-changed is a blog link, not pricing.
func pageKind(host, site string, segs []string) string {
	var words []string
	if host != site {
		words = append(words, strings.SplitN(host, ".", 2)[0])
	}
	if len(segs) > 0 {
		words = append(words, strings.ToLower(segs[0]))
	}
	if len(segs) == 2 {
		words = append(words, strings.ToLower(segs[1]))
	}
	for _, w := range words {
		switch {
		case blogSegs[w]:
			if len(segs) <= 1 {
				return KindBlog
			}
			return "" // a single post, not the blog
		case changelogSegs[w]:
			return KindChangelog
		case pricingSegs[w]:
			return KindPricing
		}
	}
	return ""
}

// FeedLink returns the page's RSS or Atom feed (absolute), or "".
func FeedLink(pageURL, body string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data == "body" {
				return ""
			}
			if t.Data != "link" {
				continue
			}
			var rel, typ, href string
			for _, a := range t.Attr {
				switch a.Key {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "type":
					typ = strings.ToLower(a.Val)
				case "href":
					href = strings.TrimSpace(a.Val)
				}
			}
			if rel == "alternate" && href != "" && (strings.Contains(typ, "rss+xml") || strings.Contains(typ, "atom+xml")) {
				if u, err := base.Parse(href); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
					return u.String()
				}
			}
		}
	}
}

// Discover fetches a company's homepage and returns where it publishes. A
// homepage that cannot be read still yields the landing page itself, so the
// competitor is watched and the failure shows up as a broken source.
func Discover(ctx context.Context, fetch onboarding.PageFetcher, website string) []Source {
	body, err := fetch(ctx, website)
	if err != nil {
		return []Source{{KindLanding, website}}
	}
	return ExtractSources(website, body)
}

// NormalizeSource validates a source a person typed and returns it in the
// form discovery would have stored: an account as its canonical profile URL,
// a page as an http(s) URL.
func NormalizeSource(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !ValidKind(kind) {
		return "", errors.New("unknown source kind")
	}
	if kind == KindX && !strings.Contains(raw, "/") {
		raw = "https://x.com/" + strings.TrimPrefix(raw, "@")
	}
	u, err := onboarding.NormalizeProductURL(raw)
	if err != nil {
		return "", err
	}
	switch kind {
	case KindX, KindLinkedIn, KindGitHub:
		// A profile link is recognised exactly as it would be on a homepage.
		for _, s := range ExtractSources("https://example.invalid/", `<a href="`+html.EscapeString(u)+`">x</a>`) {
			if s.Kind == kind {
				return s.URL, nil
			}
		}
		return "", map[string]error{
			KindX:        errors.New("enter an X profile, like x.com/acme"),
			KindLinkedIn: errors.New("enter a LinkedIn company page, like linkedin.com/company/acme"),
			KindGitHub:   errors.New("enter a GitHub organisation, like github.com/acme"),
		}[kind]
	}
	return u, nil
}
