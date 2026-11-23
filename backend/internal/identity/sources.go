package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Page is one public profile or web page read during resolution.
type Page struct {
	URL      string // the human-readable page this came from (the evidence)
	Name     string
	Bio      string
	Company  string
	Location string
	Text     string   // free text to look for links and emails in (bio, about)
	Links    []string // explicit links (profile fields, <a href>)
	MeLinks  []string // rel="me" links: the page owner says these are theirs
	Posts    []Post
	// Hints are links that are probably theirs but not proven (their own
	// Show HN launch, the original of a cross-posted article).
	Hints []Hint
	// Notes are things they said about themselves ("I work at...", "founder
	// here"): context for the research agent, never evidence.
	Notes []string
}

// Hint is a link with how much it is worth on its own.
type Hint struct {
	URL    string
	Method string
	Factor float64
}

// Post is something the person wrote, used for "what they post about".
type Post struct {
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}

// ErrNotFound means the profile does not exist.
var ErrNotFound = errors.New("identity: profile not found")

const maxBody = 1 << 20

// API bases are vars so tests can point them at a local server.
var (
	hnUserURL     = "https://hacker-news.firebaseio.com/v0/user/%s.json"
	hnPostsURL    = "https://hn.algolia.com/api/v1/search_by_date?tags=author_%s&hitsPerPage=40"
	devtoUserURL  = "https://dev.to/api/users/by_username?url=%s"
	devtoPostsURL = "https://dev.to/api/articles?username=%s&per_page=10"
	bskyURL       = "https://public.api.bsky.app/xrpc/app.bsky.actor.getProfile?actor=%s"
	githubAPI     = "https://api.github.com"
)

const userAgent = "LeadEcho-Identity/1.0 (+https://leadecho.app)"

func (r *Resolver) getJSON(ctx context.Context, rawURL string, hdr map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := r.api.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("identity: %s: status %d", req.URL.Host, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out)
}

func (r *Resolver) hackerNews(ctx context.Context, handle string) (*Page, error) {
	var u *struct {
		About string `json:"about"`
	}
	if err := r.getJSON(ctx, fmt.Sprintf(hnUserURL, url.PathEscape(handle)), nil, &u); err != nil {
		return nil, err
	}
	if u == nil { // unknown user: the API answers null
		return nil, ErrNotFound
	}
	about := htmlToText(u.About)
	p := &Page{URL: "https://news.ycombinator.com/user?id=" + url.QueryEscape(handle), Bio: clip(about, 600), Text: about}
	var posts struct {
		Hits []struct {
			Title       string `json:"title"`
			StoryTitle  string `json:"story_title"`
			URL         string `json:"url"`
			CommentText string `json:"comment_text"`
			ObjectID    string `json:"objectID"`
		} `json:"hits"`
	}
	if r.getJSON(ctx, fmt.Sprintf(hnPostsURL, url.QueryEscape(handle)), nil, &posts) == nil {
		for _, h := range posts.Hits {
			item := "https://news.ycombinator.com/item?id=" + h.ObjectID
			if t := firstNonEmpty(h.Title, h.StoryTitle); t != "" && len(p.Posts) < 12 {
				p.Posts = append(p.Posts, Post{Title: clip(t, 200), URL: item})
			}
			// Their own launch: the product is probably theirs (or their employer's).
			if showHNRe.MatchString(h.Title) && h.URL != "" {
				p.Hints = append(p.Hints, Hint{URL: h.URL, Method: "show_hn", Factor: showHNHint})
			}
			if c := htmlToText(h.CommentText); selfDisclosureRe.MatchString(c) && len(p.Notes) < maxNotes {
				p.Notes = append(p.Notes, clip(c, 400)+" ("+item+")")
			}
		}
	}
	return p, nil
}

const (
	showHNHint    = 0.7 // a candidate: shown as "possible", checked by the agent
	canonicalHint = 0.9 // the original of an article they cross-posted: where they publish
	maxNotes      = 6
)

var (
	showHNRe = regexp.MustCompile(`(?i)^(show|launch) hn\b`)
	// selfDisclosureRe finds comments where people say who they are.
	selfDisclosureRe = regexp.MustCompile(`(?i)\b(i (work|worked) (at|for|on)|i'?m (the |a )?(co-?)?(founder|ceo|cto|author|maintainer|creator|lead)|i am (the |a )?(co-?)?(founder|ceo|cto|author|maintainer|creator|lead)|(co-?)?founder here|author here|creator here|maintainer here|disclaimer:|disclosure:|my (company|startup|employer|product|team)|i (built|wrote|made|run|started)|we (built|launched|make)|our (company|startup|product))\b`)
)

func (r *Resolver) devto(ctx context.Context, handle string) (*Page, error) {
	var u struct {
		Name     string `json:"name"`
		Summary  string `json:"summary"`
		Location string `json:"location"`
		Website  string `json:"website_url"`
		Twitter  string `json:"twitter_username"`
		GitHub   string `json:"github_username"`
	}
	if err := r.getJSON(ctx, fmt.Sprintf(devtoUserURL, url.QueryEscape(handle)), nil, &u); err != nil {
		return nil, err
	}
	p := &Page{URL: "https://dev.to/" + handle, Name: u.Name, Bio: clip(u.Summary, 600), Location: u.Location, Text: u.Summary}
	// Profile fields the owner filled in: same weight as rel="me".
	if u.Website != "" {
		p.MeLinks = append(p.MeLinks, ensureScheme(u.Website))
	}
	if u.Twitter != "" {
		p.MeLinks = append(p.MeLinks, "https://x.com/"+u.Twitter)
	}
	if u.GitHub != "" {
		p.MeLinks = append(p.MeLinks, "https://github.com/"+u.GitHub)
	}
	var arts []struct {
		Title     string   `json:"title"`
		URL       string   `json:"url"`
		Canonical string   `json:"canonical_url"`
		Tags      []string `json:"tag_list"`
	}
	if r.getJSON(ctx, fmt.Sprintf(devtoPostsURL, url.QueryEscape(handle)), nil, &arts) == nil {
		for _, a := range arts {
			// A cross-post names its original: where they publish themselves.
			if a.Canonical != "" && !strings.Contains(strings.ToLower(a.Canonical), "://dev.to/") {
				p.Hints = append(p.Hints, Hint{URL: a.Canonical, Method: "canonical", Factor: canonicalHint})
			}
			title := a.Title
			if len(a.Tags) > 0 {
				title += " [" + strings.Join(a.Tags, ", ") + "]"
			}
			p.Posts = append(p.Posts, Post{Title: clip(title, 200), URL: a.URL})
		}
	}
	return p, nil
}

func (r *Resolver) bluesky(ctx context.Context, handle string) (*Page, error) {
	var u struct {
		Handle      string `json:"handle"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	}
	if err := r.getJSON(ctx, fmt.Sprintf(bskyURL, url.QueryEscape(handle)), nil, &u); err != nil {
		return nil, err
	}
	p := &Page{URL: "https://bsky.app/profile/" + u.Handle, Name: u.DisplayName, Bio: clip(u.Description, 600), Text: u.Description}
	// A custom-domain handle is proven by DNS or /.well-known: the domain is theirs.
	if h := strings.ToLower(u.Handle); h != "" && !strings.HasSuffix(h, ".bsky.social") && strings.Contains(h, ".") {
		p.MeLinks = append(p.MeLinks, "https://"+h)
	}
	return p, nil
}

func (r *Resolver) github(ctx context.Context, handle string) (*Page, error) {
	hdr := map[string]string{"Accept": "application/vnd.github+json"}
	if r.githubToken != "" {
		hdr["Authorization"] = "Bearer " + r.githubToken
	}
	var u struct {
		Login    string `json:"login"`
		Type     string `json:"type"`
		Name     string `json:"name"`
		Company  string `json:"company"`
		Blog     string `json:"blog"`
		Location string `json:"location"`
		Email    string `json:"email"` // public profile email, shown on the profile itself
		Bio      string `json:"bio"`
		Twitter  string `json:"twitter_username"`
	}
	if err := r.getJSON(ctx, githubAPI+"/users/"+url.PathEscape(handle), hdr, &u); err != nil {
		return nil, err
	}
	if u.Type == "Organization" {
		return nil, ErrNotFound // an org is not a person
	}
	p := &Page{URL: "https://github.com/" + u.Login, Name: u.Name, Bio: clip(u.Bio, 600),
		Company: strings.TrimPrefix(strings.TrimSpace(u.Company), "@"), Location: u.Location, Text: u.Bio}
	// GitHub only shows these on the profile when the owner set them, so they
	// carry the same weight as rel="me".
	if u.Blog != "" {
		p.MeLinks = append(p.MeLinks, ensureScheme(u.Blog))
	}
	if u.Twitter != "" {
		p.MeLinks = append(p.MeLinks, "https://x.com/"+u.Twitter)
	}
	if u.Email != "" {
		p.MeLinks = append(p.MeLinks, "mailto:"+u.Email)
	}
	var social []struct {
		URL string `json:"url"`
	}
	if r.getJSON(ctx, githubAPI+"/users/"+url.PathEscape(u.Login)+"/social_accounts", hdr, &social) == nil {
		for _, s := range social {
			p.MeLinks = append(p.MeLinks, s.URL)
		}
	}
	return p, nil
}

var aboutPathRe = regexp.MustCompile(`(?i)^/(about|about-me|contact|links|now)/?$`)

// website reads a personal site's home page and, when it links to one, its
// about/contact page.
func (r *Resolver) website(ctx context.Context, site string) (*Page, error) {
	home, finalURL, err := r.fetchHTML(ctx, site)
	if err != nil {
		return nil, err
	}
	p := parsePage(home, finalURL)
	base, _ := url.Parse(finalURL)
	for _, l := range p.Links {
		u, err := url.Parse(l)
		if err != nil || base == nil || !strings.EqualFold(u.Hostname(), base.Hostname()) || !aboutPathRe.MatchString(u.Path) {
			continue
		}
		if body, final, err := r.fetchHTML(ctx, u.String()); err == nil {
			sub := parsePage(body, final)
			p.Links = append(p.Links, sub.Links...)
			p.MeLinks = append(p.MeLinks, sub.MeLinks...)
			p.Text += "\n" + sub.Text
		}
		break
	}
	p.URL = finalURL
	return p, nil
}

// fetchHTML GETs a user-supplied URL through the SSRF-safe client, following
// up to three redirects (each one re-checked by the client's dialer).
func (r *Resolver) fetchHTML(ctx context.Context, raw string) (body, final string, err error) {
	cur := raw
	for hop := 0; hop < 4; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cur, nil)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html")
		resp, err := r.web.Do(req)
		if err != nil {
			return "", "", err
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc, lerr := resp.Location()
			resp.Body.Close()
			if lerr != nil || (loc.Scheme != "http" && loc.Scheme != "https") {
				return "", "", fmt.Errorf("identity: bad redirect from %s", cur)
			}
			cur = loc.String()
			continue
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if rerr != nil {
			return "", "", rerr
		}
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("identity: %s: status %d", cur, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "html") {
			return "", "", fmt.Errorf("identity: %s is not html", cur)
		}
		return string(b), cur, nil
	}
	return "", "", fmt.Errorf("identity: too many redirects from %s", raw)
}

// parsePage pulls the title, description, visible text, links and rel="me"
// links out of an HTML page. Relative links resolve against pageURL.
func parsePage(body, pageURL string) *Page {
	base, _ := url.Parse(pageURL)
	p := &Page{URL: pageURL}
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return p
	}
	var text strings.Builder
	var walk func(n *xhtml.Node, skip bool)
	walk = func(n *xhtml.Node, skip bool) {
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "svg":
				skip = true
			case "title":
				if n.FirstChild != nil && p.Name == "" {
					p.Bio = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				if name := strings.ToLower(attr(n, "name") + attr(n, "property")); name == "description" || name == "og:description" {
					if c := strings.TrimSpace(attr(n, "content")); c != "" {
						p.Bio = c
					}
				}
			case "a", "link":
				if href := resolve(base, attr(n, "href")); href != "" {
					if hasToken(attr(n, "rel"), "me") {
						p.MeLinks = append(p.MeLinks, href)
					} else if n.Data == "a" {
						p.Links = append(p.Links, href)
					}
				}
			}
		}
		if n.Type == xhtml.TextNode && !skip {
			if t := strings.TrimSpace(n.Data); t != "" {
				text.WriteString(t)
				text.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	p.Text = clip(text.String(), 20000)
	p.Bio = clip(p.Bio, 400)
	return p
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasToken(list, tok string) bool {
	for _, f := range strings.Fields(strings.ToLower(list)) {
		if f == tok {
			return true
		}
	}
	return false
}

func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(href), "mailto:") {
		return href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// htmlToText turns an HN "about" (HTML) into text, keeping link targets.
func htmlToText(s string) string {
	s = strings.NewReplacer("<p>", "\n", "<br>", "\n").Replace(s)
	s = regexp.MustCompile(`(?i)<a[^>]+href="([^"]+)"[^>]*>`).ReplaceAllString(s, " $1 ")
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}

func ensureScheme(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		return "https://" + s
	}
	return s
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
