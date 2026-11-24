package identity

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"leadecho/internal/ai"
)

// Confidence for links the research agent reports.
const (
	agentTrustedPage = 0.9 // on a page that already belongs to them, or a profile that links back
	// agentHandleCluster: a profile with the author's own username that is
	// linked both ways with another of the person's pages (GitHub <-> site).
	// One shared username is weak; a consistent pair of them is how people
	// actually recognise each other online.
	agentHandleCluster = 0.85
	agentOtherPage     = 0.7  // on a page it read that isn't known to be theirs: a candidate
	agentSearchOnly    = 0.65 // a search result or an opened profile, nothing tying it to them
	maxAgentPasses     = 4
)

// MergeAgent returns a copy of p with what the agent found that checks out.
// The model's word counts for nothing: a link is shown only when it is among
// the links of a page the agent actually read (web fetch or browser) and
// that page already belongs to the person, or when a profile it opened links
// back to something of theirs. Emails must appear on such a page exactly as
// claimed. Everything else stays a candidate below Shown; a LinkedIn
// candidate can still be confirmed by the paid person lookup.
func MergeAgent(p *Profile, f *ai.PersonFindings) *Profile {
	out := *p
	out.Links = append([]Link(nil), p.Links...)
	out.Tier, out.ResolvedAt = "b", time.Now().UTC()
	if f == nil {
		return &out
	}
	ev := newEvidence(p, f)
	ev.claimedName = f.Name
	accepted := map[int]bool{}
	for pass := 0; pass < maxAgentPasses; pass++ { // a newly trusted page can vouch for the next link
		changed := false
		for i, al := range f.Links {
			c, ok := Classify(al.URL)
			if !ok || accepted[i] {
				continue
			}
			conf := ev.vouches(c, al.Evidence)
			if conf == 0 {
				continue
			}
			accepted[i], changed = true, true
			ev.trust(c.URL)
			out.Links = upsertLink(out.Links, Link{Kind: c.Kind, URL: c.URL, Handle: c.Handle,
				Confidence: conf, Evidence: al.Evidence, Method: "agent"})
		}
		if !changed {
			break
		}
	}
	for i, al := range f.Links {
		c, ok := Classify(al.URL)
		if !ok || accepted[i] {
			continue
		}
		if conf := ev.candidate(c, al.Evidence); conf > 0 {
			out.Links = upsertLink(out.Links, Link{Kind: c.Kind, URL: c.URL, Handle: c.Handle,
				Confidence: conf, Evidence: al.Evidence, Method: "agent_candidate"})
		}
	}
	out.Works = mergeWorks(p.Works, ev.works(f))
	out.Name = firstNonEmpty(p.Name, f.Name)
	out.Role = firstNonEmpty(f.Role, p.Role)
	out.Company = firstNonEmpty(p.Company, f.Company)
	out.Location = firstNonEmpty(p.Location, f.Location)
	out.Summary = firstNonEmpty(f.Summary, p.Summary)
	if len(f.Topics) > 0 {
		out.Topics = f.Topics
	}
	return &out
}

// evidence is what the agent read, reduced to the links on each page.
type evidence struct {
	handle      string                     // the author's username where the lead came from
	name        string                     // the name tier a found ("" if none)
	claimedName string                     // the name the agent reports (trusted only if a page of theirs shows it)
	pages       map[string]map[string]bool // normalized page URL -> canonical links on it
	text        map[string]string          // normalized page URL -> lower-cased text
	trusted     map[string]bool            // canonical URLs that are theirs
	hosts       map[string]bool            // hosts of their websites
	searched    map[string]bool            // normalized search-result URLs
}

func newEvidence(p *Profile, f *ai.PersonFindings) *evidence {
	e := &evidence{name: p.Name, pages: map[string]map[string]bool{}, text: map[string]string{},
		trusted: map[string]bool{}, hosts: map[string]bool{}, searched: map[string]bool{}}
	for u, text := range f.Seen {
		links := map[string]bool{}
		for _, c := range ExtractLinks(text) {
			links[c.URL] = true
		}
		e.pages[normalize(u)] = links
		e.text[normalize(u)] = strings.ToLower(text)
	}
	for u := range f.SearchURLs {
		e.searched[normalize(u)] = true
	}
	for _, l := range p.Links {
		if l.Confidence >= Shown {
			e.trust(l.URL)
		}
		if l.Method == "origin" {
			e.handle = strings.ToLower(l.Handle)
		}
	}
	return e
}

// personTokens are words that name this person: their username, the name
// tier a found, and the agent's name once a profile page of theirs shows it.
func (e *evidence) personTokens() []string {
	tokens := nameTokens(e.name)
	if t := handleToken(e.handle); t != "" {
		tokens = append(tokens, t)
	}
	if n := strings.ToLower(strings.TrimSpace(e.claimedName)); n != "" {
		for page, text := range e.text {
			if c, ok := Classify("https://" + page); ok && c.Kind != KindWebsite && e.trusted[c.URL] && strings.Contains(text, n) {
				return append(tokens, nameTokens(n)...)
			}
		}
	}
	return tokens
}

// personal: a contact link taken from a website names the person, unless the
// site is named after their username. An employer's /team page lists
// colleagues; their LinkedIn or email must not become this person's.
func (e *evidence) personal(c Classified, page string) bool {
	pc, ok := Classify(page)
	if !ok || pc.Kind != KindWebsite || e.ownHandle(pc) {
		return true
	}
	l := Link{Kind: c.Kind, URL: c.URL, Handle: c.Handle}
	return mentionsAny(l, e.personTokens()) || ownerMailbox(l, page)
}

// ownHandle: a profile under the author's username, or a site named after it.
func (e *evidence) ownHandle(c Classified) bool {
	if len(e.handle) < 4 {
		return false // "bob" is everyone
	}
	if c.Kind == KindWebsite {
		return len(e.handle) >= 5 && strings.Contains(strings.ToLower(c.URL), e.handle)
	}
	h := strings.ToLower(c.Handle)
	return h == e.handle || strings.HasPrefix(h, e.handle+"@") // mastodon: user@host
}

// mutual reports whether page (one of the claimed profiles, read) links to
// another read page that links back to it.
func (e *evidence) mutual(c Classified, links map[string]bool) bool {
	for l := range links {
		if l == c.URL {
			continue
		}
		for page, back := range e.pages {
			if pc, ok := Classify("https://" + page); ok && pc.URL == l && back[c.URL] {
				return true
			}
		}
	}
	return false
}

func (e *evidence) trust(u string) {
	e.trusted[u] = true
	if c, ok := Classify(u); ok && c.Kind == KindWebsite {
		e.hosts[strings.TrimPrefix(c.URL, "https://")] = true
	}
}

// theirs reports whether a page URL belongs to the person: one of their
// profiles, or any page on one of their sites.
func (e *evidence) theirs(page string) bool {
	c, ok := Classify(page)
	if !ok {
		return false
	}
	if e.trusted[c.URL] {
		return true
	}
	return c.Kind == KindWebsite && e.hosts[strings.TrimPrefix(c.URL, "https://")]
}

func (e *evidence) linksOn(page string) map[string]bool { return e.pages[normalize(page)] }

// vouches returns the confidence the evidence gives a claimed link (0 when
// none):
//   - a page of theirs links to it (a site page only if the link names them);
//   - or the agent read the claimed profile, it links to something of theirs,
//     and something of theirs links to it (both ways);
//   - or it is under the author's own username and linked with another such
//     profile or a page it links both ways with.
//
// A role mailbox (support@, info@) is never theirs.
func (e *evidence) vouches(c Classified, page string) float64 {
	if c.Kind == KindEmail && roleAddressRe.MatchString(c.Handle) {
		return 0
	}
	if e.linksOn(page)[c.URL] && e.theirs(page) && e.personal(c, page) {
		return agentTrustedPage
	}
	self := e.ownPage(c)
	if self == nil || c.Kind == KindEmail {
		return 0
	}
	if e.linkedFromTheirs(c) {
		for l := range self {
			if l != c.URL && (e.trusted[l] || e.theirs(l)) {
				return agentTrustedPage
			}
		}
	}
	if e.ownHandle(c) && (e.mutual(c, self) || e.handleLinked(c, self)) {
		return agentHandleCluster
	}
	return 0
}

// linkedFromTheirs: some read page of theirs links to c (and, on a site, the
// link names them).
func (e *evidence) linkedFromTheirs(c Classified) bool {
	for page, links := range e.pages {
		if links[c.URL] && e.theirs("https://"+page) && e.personal(c, "https://"+page) {
			return true
		}
	}
	return false
}

// handleLinked: c and another profile under the author's username point at
// each other in at least one direction (mastodon.social/@jane -> jane.dev).
func (e *evidence) handleLinked(c Classified, self map[string]bool) bool {
	for page, links := range e.pages {
		pc, ok := Classify("https://" + page)
		if !ok || pc.URL == c.URL || !e.ownHandle(pc) {
			continue
		}
		if links[c.URL] || self[pc.URL] {
			return true
		}
	}
	return false
}

// ownPage returns the links of the claimed profile's own page, if the agent
// read it (for a website, any page on that site).
func (e *evidence) ownPage(c Classified) map[string]bool {
	for page, links := range e.pages {
		if pc, ok := Classify("https://" + page); ok && pc.URL == c.URL {
			return links
		}
	}
	return nil
}

func (e *evidence) candidate(c Classified, page string) float64 {
	links := e.linksOn(page)
	switch {
	case links[c.URL] && c.Kind != KindEmail:
		return agentOtherPage
	case c.Kind == KindEmail:
		return 0 // an address on a page not known to be theirs is not their address
	case e.searched[normalize(c.URL)]:
		return agentSearchOnly
	}
	if e.ownPage(c) != nil {
		return agentSearchOnly // a profile it opened, with nothing tying it to them
	}
	return 0
}

// upsertLink returns links with l added, or with the existing entry for the
// same URL raised to l's confidence.
func upsertLink(links []Link, l Link) []Link {
	for i, x := range links {
		if x.URL == l.URL {
			if l.Confidence > x.Confidence {
				out := append([]Link(nil), links...)
				out[i] = l
				return out
			}
			return links
		}
	}
	return append(links, l)
}

// normalize makes "https://Jane.dev/about/" and "http://jane.dev/about" one key.
// ponytail: a search-result URL with tracking params won't match its clean form.
func normalize(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/"))
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	return host + strings.TrimRight(u.EscapedPath(), "/") + query(u)
}

func query(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	return "?" + u.RawQuery
}

const maxWorks = 20

// works keeps the published works that are provably theirs: on their own
// site or under a verified profile, under their verified username on a
// shared platform (dev.to/<them>/..., medium.com/@<them>/...), or on a page
// the agent read that names them in full (a talk or podcast page).
func (e *evidence) works(f *ai.PersonFindings) []Work {
	handles := e.trustedHandles()
	fullName := strings.ToLower(strings.Join(strings.Fields(firstNonEmpty(e.name, e.verifiedName())), " "))
	var out []Work
	for _, w := range f.Works {
		u, err := url.Parse(w.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		path := strings.ToLower(u.Path)
		ok := e.theirs(w.URL)
		for _, h := range handles {
			ok = ok || strings.HasPrefix(path, "/"+h+"/") || strings.HasPrefix(path, "/@"+h+"/") ||
				strings.HasPrefix(strings.ToLower(u.Hostname()), h+".")
		}
		if !ok && strings.Contains(fullName, " ") {
			ok = strings.Contains(e.text[normalize(w.URL)], fullName)
		}
		if ok {
			out = append(out, Work{Kind: w.Kind, Title: w.Title, URL: w.URL, Date: w.Date, Evidence: w.Evidence})
		}
	}
	return out
}

// trustedHandles are their usernames on verified profiles (4+ characters:
// short ones match too many paths).
func (e *evidence) trustedHandles() []string {
	seen := map[string]bool{}
	var out []string
	for u := range e.trusted {
		if c, ok := Classify(u); ok && c.Kind != KindWebsite && c.Kind != KindEmail && c.Kind != KindBooking {
			h := strings.ToLower(strings.SplitN(c.Handle, "@", 2)[0])
			if len(h) >= 4 && !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// verifiedName is the agent's name once a profile page of theirs shows it.
func (e *evidence) verifiedName() string {
	n := strings.ToLower(strings.TrimSpace(e.claimedName))
	if n == "" {
		return ""
	}
	for page, text := range e.text {
		if c, ok := Classify("https://" + page); ok && c.Kind != KindWebsite && e.trusted[c.URL] && strings.Contains(text, n) {
			return e.claimedName
		}
	}
	return ""
}

// mergeWorks adds new works after the ones already known, without
// duplicates, most recent first within the new batch, capped.
func mergeWorks(old, add []Work) []Work {
	out := append([]Work(nil), old...)
	seen := map[string]bool{}
	for _, w := range out {
		seen[normalize(w.URL)] = true
	}
	sort.SliceStable(add, func(i, j int) bool { return add[i].Date > add[j].Date })
	for _, w := range add {
		if k := normalize(w.URL); !seen[k] && len(out) < maxWorks {
			seen[k] = true
			out = append(out, w)
		}
	}
	return out
}
