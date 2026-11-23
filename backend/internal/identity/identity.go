// Package identity resolves a forum handle (HN, Reddit, Dev.to, Bluesky, X)
// to the person behind it from public pages only: their own profile, the
// sites and profiles it links to, and the profiles that link back.
//
// Every link carries the page that proves it and a confidence. A link is
// shown, and handed to paid enrichment, only at Shown (0.8) or above: a
// wrong person is worse than no person. A shared username alone never gets
// there; the person's own profile pointing at it, or two profiles pointing
// at each other, does.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"leadecho/internal/netsafe"
)

const (
	// Shown is the confidence from which a link is shown and used.
	Shown = 0.8

	maxFetches = 8
	maxDepth   = 2
	cacheTTL   = 14 * 24 * time.Hour
	cacheKeyV  = "identity:v1:"
)

// Confidence factors.
const (
	ownerLink    = 0.97 // rel="me", a profile field the owner filled in
	bioLink      = 0.95 // a link in the bio the owner wrote
	siteLink     = 0.85 // the only link of its kind on their own site
	repoOwner    = 0.55 // the owner of a repository their profile links to
	sameHandle   = 0.35 // a profile elsewhere with the same username
	sameName     = 0.6  // ...whose display name also matches
	roleAddress  = 0.5  // factor for security@, info@, ...
	linksBack    = 0.95 // a profile that links back to one we already trust
	originWeight = 1.0
)

// Link is one way to find or reach the person.
type Link struct {
	Kind       string  `json:"kind"`
	URL        string  `json:"url"`
	Handle     string  `json:"handle,omitempty"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"` // the page where it was found
	Method     string  `json:"method"`   // owner_link|bio_link|site_link|links_back|same_handle|repo_owner|origin|agent
}

// Profile is everything public found about one person.
type Profile struct {
	Name     string `json:"name,omitempty"`
	Bio      string `json:"bio,omitempty"`
	Company  string `json:"company,omitempty"`
	Role     string `json:"role,omitempty"`
	Location string `json:"location,omitempty"`
	// Summary and Topics come from the research agent (tier b).
	Summary     string    `json:"summary,omitempty"`
	Topics      []string  `json:"topics,omitempty"`
	Links       []Link    `json:"links"`
	RecentPosts []Post    `json:"recent_posts,omitempty"`
	Tier        string    `json:"tier"` // "a" (public profiles) or "b" (research agent)
	Fetched     []string  `json:"fetched,omitempty"`
	ResolvedAt  time.Time `json:"resolved_at"`
	// Incomplete: a source failed (rate limit, outage), so this is not the
	// whole answer. It is used but not cached or stored as final.
	Incomplete bool `json:"-"`
}

// Best returns the most confident link of a kind at or above Shown.
func (p *Profile) Best(kind string) (Link, bool) {
	var best Link
	for _, l := range p.Links {
		if l.Kind == kind && l.Confidence >= Shown && l.Confidence > best.Confidence {
			best = l
		}
	}
	return best, best.URL != ""
}

// HasShown reports whether any link (other than the starting profile) is shown.
func (p *Profile) HasShown() bool {
	for _, l := range p.Links {
		if l.Method != "origin" && l.Confidence >= Shown {
			return true
		}
	}
	return false
}

// Email is the best published address ("" when none).
func (p *Profile) Email() string {
	if l, ok := p.Best(KindEmail); ok {
		return l.Handle
	}
	return ""
}

// Complete reports whether there is nothing left for the agent to find:
// an email and a LinkedIn profile, both at Shown.
func (p *Profile) Complete() bool {
	_, li := p.Best(KindLinkedIn)
	return li && p.Email() != ""
}

// Seed is what the lead says about its author.
type Seed struct {
	Platform   string // reddit|hackernews|devto|bluesky|twitter|...
	Handle     string
	ProfileURL string
}

// Cache is the slice of the shared Redis cache the resolver uses
// (treg.RedisCache satisfies it). Profiles are public, so one cached
// resolution serves every workspace.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
}

// XProfileFunc reads an X profile (paid, through treg). Optional.
type XProfileFunc func(ctx context.Context, handle string) (*Page, error)

// Resolver runs tier a. Safe for concurrent use.
type Resolver struct {
	api         *http.Client // known public APIs
	web         *http.Client // user-supplied URLs: SSRF-safe
	githubToken string
	cache       Cache
	xProfile    XProfileFunc
}

// New returns a resolver. githubToken may be "" (60 GitHub calls an hour).
func New(githubToken string, cache Cache) *Resolver {
	web := netsafe.Client(12 * time.Second)
	// Hand redirects back to fetchHTML, which follows a few: each hop is a new
	// request through the same guarded dialer.
	web.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Resolver{
		api:         netsafe.Client(12 * time.Second),
		web:         web,
		githubToken: githubToken,
		cache:       cache,
	}
}

// SetXProfile lets the resolver start from an X handle.
func (r *Resolver) SetXProfile(f XProfileFunc) { r.xProfile = f }

func cacheKey(s Seed) string {
	return cacheKeyV + strings.ToLower(s.Platform) + ":" + strings.ToLower(s.Handle)
}

// Cached returns a stored profile for the seed, if any.
func (r *Resolver) Cached(ctx context.Context, s Seed) (*Profile, bool) {
	if r == nil || r.cache == nil {
		return nil, false
	}
	b, ok := r.cache.Get(ctx, cacheKey(s))
	if !ok {
		return nil, false
	}
	var p Profile
	if json.Unmarshal(b, &p) != nil {
		return nil, false
	}
	return &p, true
}

// Store caches a profile for every workspace that meets this author.
func (r *Resolver) Store(ctx context.Context, s Seed, p *Profile) {
	if r == nil || r.cache == nil || p == nil || p.Incomplete {
		return
	}
	if b, err := json.Marshal(p); err == nil {
		r.cache.Set(ctx, cacheKey(s), b, cacheTTL)
	}
}

// ErrUnsupported: no public profile to start from on this platform.
var ErrUnsupported = errors.New("identity: platform not supported")

// Resolve walks out from the author's own profile. It never fails on a
// single source: whatever was found is returned.
func (r *Resolver) Resolve(ctx context.Context, s Seed) (*Profile, error) {
	if !slugRe.MatchString(s.Handle) {
		return nil, ErrUnsupported
	}
	if p, ok := r.Cached(ctx, s); ok {
		return p, nil
	}
	kind, ok := originKind[strings.ToLower(s.Platform)]
	if !ok || (kind == KindX && r.xProfile == nil) {
		return nil, ErrUnsupported
	}
	w := newWalk(r)
	origin, _ := Classify(originURL(kind, s.Handle))
	origin.Handle = s.Handle
	w.add(origin, originWeight, "", "origin")
	w.run(ctx)
	w.guessSameHandle(ctx, s.Handle)
	p := w.profile()
	r.Store(ctx, s, p)
	return p, nil
}

var originKind = map[string]string{
	"hackernews": KindHackerNews, "reddit": KindReddit, "devto": KindDevto,
	"bluesky": KindBluesky, "twitter": KindX, "x": KindX, "github": KindGitHub,
}

func originURL(kind, handle string) string {
	switch kind {
	case KindHackerNews:
		return "https://news.ycombinator.com/user?id=" + handle
	case KindReddit:
		return "https://www.reddit.com/user/" + handle
	case KindDevto:
		return "https://dev.to/" + handle
	case KindBluesky:
		return "https://bsky.app/profile/" + handle
	case KindX:
		return "https://x.com/" + handle
	}
	return "https://github.com/" + handle
}

// ---- the walk ----------------------------------------------------------

type node struct {
	Link
	depth int
	page  *Page
	done  bool
}

type walk struct {
	r       *Resolver
	nodes   map[string]*node // by URL
	order   []string
	fetches int
	failed  bool // a fetch failed for a reason other than "not found"
}

func newWalk(r *Resolver) *walk { return &walk{r: r, nodes: map[string]*node{}} }

// add records a candidate link, keeping the highest confidence seen.
func (w *walk) add(c Classified, conf float64, evidence, method string) *node {
	if c.URL == "" {
		return nil
	}
	if c.FromRepo {
		conf, method = conf*repoOwner, "repo_owner"
	}
	if c.Kind == KindEmail && roleAddressRe.MatchString(c.Handle) {
		conf *= roleAddress // security@, support@: a mailbox, not the person
	}
	n, ok := w.nodes[c.URL]
	if !ok {
		n = &node{Link: Link{Kind: c.Kind, URL: c.URL, Handle: c.Handle}}
		w.nodes[c.URL] = n
		w.order = append(w.order, c.URL)
	}
	if conf > n.Confidence {
		n.Confidence, n.Evidence, n.Method = conf, evidence, method
	}
	return n
}

func fetchable(kind string) bool {
	switch kind {
	case KindHackerNews, KindReddit, KindDevto, KindBluesky, KindGitHub, KindWebsite, KindX:
		return true
	}
	return false
}

// run fetches trusted, unfetched nodes breadth-first until the budget ends.
func (w *walk) run(ctx context.Context) {
	for depth := 0; depth <= maxDepth; depth++ {
		for _, u := range append([]string(nil), w.order...) {
			n := w.nodes[u]
			if n.done || n.depth > depth || n.Confidence < Shown || !fetchable(n.Kind) || w.fetches >= maxFetches {
				continue
			}
			w.visit(ctx, n)
		}
	}
}

func (w *walk) fetch(ctx context.Context, n *node) (*Page, error) {
	w.fetches++
	switch n.Kind {
	case KindHackerNews:
		return w.r.hackerNews(ctx, n.Handle)
	case KindReddit:
		return w.r.reddit(ctx, n.Handle)
	case KindDevto:
		return w.r.devto(ctx, n.Handle)
	case KindBluesky:
		return w.r.bluesky(ctx, n.Handle)
	case KindGitHub:
		return w.r.github(ctx, n.Handle)
	case KindX:
		if w.r.xProfile == nil {
			return nil, ErrUnsupported
		}
		return w.r.xProfile(ctx, n.Handle)
	}
	return w.r.website(ctx, n.URL)
}

// visit fetches a node and adds what its page links to.
func (w *walk) visit(ctx context.Context, n *node) {
	n.done = true
	page, err := w.fetch(ctx, n)
	switch {
	case errors.Is(err, ErrNotFound) && n.Method != "origin":
		n.Confidence = 0 // a dead link or an organisation account: not theirs
		return
	case err != nil:
		w.failed = true
		return
	case page == nil:
		return
	}
	w.process(n, page)
}

// process records a fetched page: it may vouch for its own node (it links
// back to something trusted) and it adds the links it holds.
func (w *walk) process(n *node, page *Page) {
	n.page = page
	evidence := firstNonEmpty(page.URL, n.URL)
	// Links back to something already trusted make this page trusted too.
	if w.linksBack(n, page) {
		n.Confidence, n.Method = max(n.Confidence, linksBack), firstNonEmpty(keepOrigin(n.Method), "links_back")
	}
	conf := n.Confidence
	child := func(c Classified, factor float64, method string) {
		if c.URL == n.URL {
			return
		}
		if m := w.add(c, conf*factor, evidence, method); m != nil && m.depth == 0 && m.Method != "origin" {
			m.depth = n.depth + 1
		}
	}
	for _, l := range page.MeLinks {
		if c, ok := Classify(l); ok {
			child(c, ownerLink, "owner_link")
		}
	}
	if n.Kind == KindWebsite {
		w.siteLinks(page, child)
		return
	}
	for _, l := range page.Links {
		if c, ok := Classify(l); ok {
			child(c, bioLink, "bio_link")
		}
	}
	for _, c := range ExtractLinks(page.Text) {
		child(c, bioLink, "bio_link")
	}
}

func keepOrigin(m string) string {
	if m == "origin" {
		return m
	}
	return ""
}

// siteLinks takes links from a personal site with care: a blog links to
// other people's profiles too. A profile link counts only when it is the
// only one of its kind on the page; booking links and emails at the site's
// own domain always count.
func (w *walk) siteLinks(page *Page, child func(Classified, float64, string)) {
	byKind := map[string][]Classified{}
	seen := map[string]bool{}
	all := append([]string(nil), page.Links...)
	for _, c := range ExtractLinks(page.Text) {
		all = append(all, c.URL)
	}
	for _, l := range all {
		c, ok := Classify(l)
		if !ok || seen[c.URL] || c.Kind == KindWebsite {
			continue
		}
		seen[c.URL] = true
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	site := hostOf(page.URL)
	for kind, cs := range byKind {
		for _, c := range cs {
			switch {
			case kind == KindBooking:
				child(c, siteLink, "site_link")
			case kind == KindEmail && (sameSite(emailDomain(c.Handle), site) || len(cs) == 1):
				child(c, siteLink, "site_link")
			case kind != KindEmail && len(cs) == 1:
				child(c, siteLink, "site_link")
			}
		}
	}
}

// linksBack reports whether page links to a node (other than n) that is
// already trusted: two profiles pointing at each other are one person.
func (w *walk) linksBack(n *node, page *Page) bool {
	if n.Method == "origin" {
		return false
	}
	var urls []string
	urls = append(urls, page.MeLinks...)
	urls = append(urls, page.Links...)
	for _, c := range ExtractLinks(page.Text) {
		urls = append(urls, c.URL)
	}
	for _, l := range urls {
		c, ok := Classify(l)
		if !ok || c.URL == n.URL {
			continue
		}
		if m := w.nodes[c.URL]; m != nil && m != n && m.Confidence >= Shown && m.Evidence != n.URL {
			return true
		}
	}
	return false
}

// guessSameHandle tries github.com/<handle> when nothing trusted pointed to
// a GitHub account. It is shown only if it links back to something trusted.
func (w *walk) guessSameHandle(ctx context.Context, handle string) {
	for _, n := range w.nodes {
		if n.Kind == KindGitHub && n.Confidence >= Shown {
			return
		}
	}
	c, ok := Classify("https://github.com/" + handle)
	if !ok || !githubLoginRe.MatchString(handle) || w.fetches >= maxFetches {
		return
	}
	n := w.add(c, sameHandle, "https://github.com/"+handle, "same_handle")
	if n.done {
		return
	}
	n.done = true
	page, err := w.fetch(ctx, n)
	if err != nil || page == nil {
		w.failed = w.failed || (err != nil && !errors.Is(err, ErrNotFound))
		return
	}
	w.process(n, page) // links back -> trusted, and its links are added
	if n.Confidence < Shown && namesMatch(page.Name, w.knownName()) {
		n.Confidence = sameName
	}
	w.run(ctx) // follow whatever it made trusted
}

var roleAddressRe = regexp.MustCompile(`(?i)^(security|abuse|privacy|support|help|info|press|media|legal|admin|webmaster|postmaster|sales|billing|jobs|careers|team|dmca|noreply|no-reply)@`)

var githubLoginRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

func (w *walk) knownName() string {
	best, conf := "", 0.0
	for _, n := range w.nodes {
		if n.page != nil && n.Confidence >= Shown && n.page.Name != "" && n.Confidence > conf {
			best, conf = n.page.Name, n.Confidence
		}
	}
	return best
}

// namesMatch: two display names that are the same full name.
func namesMatch(a, b string) bool {
	a, b = strings.ToLower(strings.Join(strings.Fields(a), " ")), strings.ToLower(strings.Join(strings.Fields(b), " "))
	return a != "" && a == b && strings.Contains(a, " ")
}

// profile assembles the result from trusted pages.
func (w *walk) profile() *Profile {
	p := &Profile{Tier: "a", ResolvedAt: time.Now().UTC(), Links: []Link{}}
	var pages []*node
	for _, u := range w.order {
		n := w.nodes[u]
		if n.Confidence > 0.3 {
			p.Links = append(p.Links, n.Link)
		}
		if n.page != nil {
			p.Fetched = append(p.Fetched, n.URL)
			if n.Confidence >= Shown {
				pages = append(pages, n)
			}
		}
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Confidence > pages[j].Confidence })
	sort.SliceStable(p.Links, func(i, j int) bool { return p.Links[i].Confidence > p.Links[j].Confidence })
	for _, n := range pages {
		pg := n.page
		if p.Name == "" && n.Kind != KindWebsite {
			p.Name = strings.TrimSpace(pg.Name)
		}
		if p.Bio == "" && n.Kind != KindWebsite {
			p.Bio = pg.Bio
		}
		p.Company = firstNonEmpty(p.Company, pg.Company)
		p.Location = firstNonEmpty(p.Location, pg.Location)
		for _, post := range pg.Posts {
			if len(p.RecentPosts) < 10 && !hasPost(p.RecentPosts, post.Title) {
				p.RecentPosts = append(p.RecentPosts, post) // HN lists a story once per comment
			}
		}
	}
	if p.Bio == "" && len(pages) > 0 {
		p.Bio = pages[0].page.Bio
	}
	p.Links = w.checkSiteLinks(p.Links, p.Name)
	p.Incomplete = w.failed
	return p
}

// checkSiteLinks: a bio can link to an employer's or a friend's site, whose
// only email or LinkedIn is someone else's. A contact link taken from a site
// stays shown only when it names the person (their name or handle) or the
// site itself is declared theirs (rel="me", a profile field) or links back.
func (w *walk) checkSiteLinks(links []Link, name string) []Link {
	tokens := nameTokens(name)
	for _, n := range w.nodes {
		if n.Kind != KindHackerNews && n.Kind != KindReddit && n.Kind != KindDevto && n.Kind != KindBluesky && n.Kind != KindGitHub && n.Kind != KindX {
			continue
		}
		if n.Method == "origin" || n.Confidence >= Shown {
			tokens = append(tokens, strings.ToLower(n.Handle))
		}
	}
	out := make([]Link, len(links))
	for i, l := range links {
		if l.Method == "site_link" && !w.siteIsTheirs(l.Evidence) && !mentionsAny(l, tokens) {
			l.Confidence = min(l.Confidence, 0.7)
		}
		out[i] = l
	}
	return out
}

func (w *walk) siteIsTheirs(evidence string) bool {
	c, ok := Classify(evidence)
	if !ok {
		return false
	}
	n := w.nodes[c.URL]
	return n != nil && (n.Method == "owner_link" || n.Method == "links_back")
}

func nameTokens(name string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(name)) {
		if len([]rune(f)) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

func mentionsAny(l Link, tokens []string) bool {
	v := strings.ToLower(firstNonEmpty(l.Handle, l.URL))
	if l.Kind == KindEmail {
		v = strings.SplitN(v, "@", 2)[0]
	}
	for _, t := range tokens {
		if t != "" && strings.Contains(v, t) {
			return true
		}
	}
	return false
}

func hasPost(posts []Post, title string) bool {
	for _, p := range posts {
		if p.Title == title {
			return true
		}
	}
	return false
}

func hostOf(u string) string {
	c, ok := Classify(u)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(c.URL, "https://")
}

func emailDomain(e string) string {
	if i := strings.LastIndex(e, "@"); i >= 0 {
		return strings.ToLower(e[i+1:])
	}
	return ""
}

// sameSite: "jane.dev" and "mail.jane.dev" are the same site.
func sameSite(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}
