package identity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		in, kind, url string
		ok            bool
	}{
		{"https://github.com/Jane", KindGitHub, "https://github.com/Jane", true},
		{"https://github.com/jane/proj", KindGitHub, "https://github.com/jane", true},
		{"https://github.com/features", "", "", false},
		{"https://www.linkedin.com/in/Jane-Doe/", KindLinkedIn, "https://www.linkedin.com/in/jane-doe", true},
		{"https://linkedin.com/company/acme", "", "", false},
		{"https://twitter.com/jane", KindX, "https://x.com/jane", true},
		{"https://x.com/i/status/1", "", "", false},
		{"https://cal.com/jane/30min", KindBooking, "https://cal.com/jane/30min", true},
		{"https://calendly.com/", "", "", false},
		{"https://jane.dev/blog/post?x=1", KindWebsite, "https://jane.dev", true},
		{"https://jane.dev/logo.png", "", "", false},
		{"https://news.ycombinator.com/item?id=1", "", "", false},
		{"https://hachyderm.io/@jane", KindMastodon, "https://hachyderm.io/@jane", true},
		{"mailto:Jane@Jane.dev?subject=hi", KindEmail, "mailto:jane@jane.dev", true},
		{"ftp://jane.dev", "", "", false},
	}
	for _, tt := range tests {
		c, ok := Classify(tt.in)
		if ok != tt.ok || c.Kind != tt.kind || c.URL != tt.url {
			t.Errorf("Classify(%q) = %+v %v, want %s %s %v", tt.in, c, ok, tt.kind, tt.url, tt.ok)
		}
	}
	if c, _ := Classify("https://github.com/jane/proj"); !c.FromRepo {
		t.Error("repo link should be marked FromRepo")
	}
}

func TestExtractLinksObfuscatedEmail(t *testing.T) {
	got := ExtractLinks("reach me: jane [at] jane [dot] dev or see jane.dev")
	found := false
	for _, c := range got {
		found = found || c.URL == "mailto:jane@jane.dev"
	}
	if !found {
		t.Fatalf("obfuscated email not found in %+v", got)
	}
}

// fakeWeb serves HN, GitHub and personal sites for one test.
type fakeWeb struct {
	hnAbout  string
	ghUsers  map[string]string // login -> JSON
	sites    map[string]string // path prefix on the server -> HTML
	srv      *httptest.Server
	requests []string
	// redirectRoot: "/" redirects to "/home" (apex -> www, http -> https)
	redirectRoot bool
}

func (f *fakeWeb) start(t *testing.T) *Resolver {
	t.Helper()
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.URL.Path)
		switch {
		case strings.HasPrefix(r.URL.Path, "/hn/user/"):
			fmt.Fprintf(w, `{"about":%q}`, f.hnAbout)
		case strings.HasPrefix(r.URL.Path, "/hn/posts"):
			fmt.Fprint(w, `{"hits":[{"title":"Show HN: my tool","objectID":"1"}]}`)
		case r.URL.Path == "/" && f.redirectRoot:
			http.Redirect(w, r, "/home", http.StatusMovedPermanently)
		case r.URL.Path == "/hn/broken":
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasPrefix(r.URL.Path, "/gh/users/"):
			rest := strings.TrimPrefix(r.URL.Path, "/gh/users/")
			if strings.HasSuffix(rest, "/social_accounts") {
				fmt.Fprint(w, `[]`)
				return
			}
			if body, ok := f.ghUsers[rest]; ok {
				fmt.Fprint(w, body)
				return
			}
			http.NotFound(w, r)
		default:
			for prefix, html := range f.sites {
				if strings.HasPrefix(r.URL.Path, prefix) {
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, html)
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	setURL(t, &hnUserURL, f.srv.URL+"/hn/user/%s.json")
	setURL(t, &hnPostsURL, f.srv.URL+"/hn/posts?a=%s")
	setURL(t, &githubAPI, f.srv.URL+"/gh")
	r := New("", nil)
	r.api, r.web = f.srv.Client(), f.srv.Client() // the SSRF guard refuses loopback
	return r
}

func setURL(t *testing.T, v *string, to string) {
	old := *v
	*v = to
	t.Cleanup(func() { *v = old })
}

func linkConf(p *Profile, url string) float64 {
	for _, l := range p.Links {
		if l.URL == url {
			return l.Confidence
		}
	}
	return 0
}

func TestResolveFollowsOwnSiteAndKeepsOnlyUniqueSiteLinks(t *testing.T) {
	f := &fakeWeb{ghUsers: map[string]string{
		"jane": `{"login":"jane","type":"User","name":"Jane Doe","company":"@acme","blog":""}`,
	}}
	r := f.start(t)
	site := f.srv.URL
	f.hnAbout = `Building things. <a href="` + site + `/">my site</a>`
	f.sites = map[string]string{"/": `<html><head><title>Jane</title></head><body>
		<a href="https://github.com/jane">GitHub</a>
		<a href="https://www.linkedin.com/in/jane-doe">LinkedIn</a>
		<a href="https://cal.com/jane/intro">Book a call</a>
		<a href="mailto:security@other.org">report</a>
		<p>Post: thanks to <a href="https://x.com/alice">Alice</a> and <a href="https://x.com/bob">Bob</a></p>
	</body></html>`}

	p, err := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Best(KindLinkedIn); !ok {
		t.Errorf("LinkedIn from own site should be shown: %+v", p.Links)
	}
	if _, ok := p.Best(KindBooking); !ok {
		t.Error("booking link should be shown")
	}
	if _, ok := p.Best(KindX); ok {
		t.Error("two different X links on a blog page must not be taken as theirs")
	}
	if p.Email() != "" {
		t.Errorf("a role address at another domain is not their email, got %q", p.Email())
	}
	if p.Name != "Jane Doe" || p.Company != "acme" {
		t.Errorf("name/company from trusted GitHub: got %q / %q", p.Name, p.Company)
	}
	if len(p.RecentPosts) == 0 {
		t.Error("recent HN posts should be kept")
	}
}

func TestSameHandleGuessShownOnlyWhenItLinksBack(t *testing.T) {
	for _, tc := range []struct {
		name, blog string
		shown      bool
	}{
		{"links back to their site", "SITE", true},
		{"unrelated namesake", "https://someone-else.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeWeb{}
			r := f.start(t)
			site := f.srv.URL
			blog := strings.ReplaceAll(tc.blog, "SITE", site)
			f.ghUsers = map[string]string{"janed": `{"login":"janed","type":"User","name":"J","blog":"` + blog + `"}`}
			f.hnAbout = "my site: " + site + "/"
			f.sites = map[string]string{"/": `<html><body>hello</body></html>`}
			p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
			conf := linkConf(p, "https://github.com/janed")
			if (conf >= Shown) != tc.shown {
				t.Fatalf("github confidence %.2f, shown want %v", conf, tc.shown)
			}
		})
	}
}

func TestRoleAddressIsNotTheirEmail(t *testing.T) {
	f := &fakeWeb{}
	r := f.start(t)
	f.hnAbout = "security@jane.dev or jane@jane.dev"
	p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if p.Email() != "jane@jane.dev" {
		t.Fatalf("email = %q, want jane@jane.dev", p.Email())
	}
}

func TestResolveRejectsBadHandleAndUnknownPlatform(t *testing.T) {
	r := New("", nil)
	if _, err := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "../etc"}); err == nil {
		t.Error("bad handle should fail")
	}
	if _, err := r.Resolve(context.Background(), Seed{Platform: "linkedin", Handle: "jane"}); err == nil {
		t.Error("linkedin is not resolved here (treg handles it)")
	}
}

func TestWebsiteRedirectIsFollowed(t *testing.T) {
	f := &fakeWeb{redirectRoot: true}
	r := f.start(t)
	f.hnAbout = "site: " + f.srv.URL + "/"
	f.sites = map[string]string{"/home": `<html><body><a href="https://cal.com/janed/intro">book</a></body></html>`}
	p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if _, ok := p.Best(KindBooking); !ok {
		t.Fatalf("redirected site not read: %+v", p.Links)
	}
}

func TestSiteContactMustNameThePerson(t *testing.T) {
	// Their bio links to their employer's site; its only email and LinkedIn
	// belong to someone else.
	f := &fakeWeb{}
	r := f.start(t)
	f.hnAbout = "I work at " + f.srv.URL + "/"
	f.sites = map[string]string{"/": `<html><body>
		<a href="mailto:ceo@acme.io">CEO</a> <a href="https://www.linkedin.com/in/boss-person">boss</a>
	</body></html>`}
	p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if p.Email() != "" {
		t.Errorf("an employer site's only email is not theirs, got %q", p.Email())
	}
	if l, ok := p.Best(KindLinkedIn); ok {
		t.Errorf("an employer site's LinkedIn is not theirs, got %s", l.URL)
	}
}

func TestOrganisationAndDeadLinksAreDropped(t *testing.T) {
	f := &fakeWeb{ghUsers: map[string]string{"acme": `{"login":"acme","type":"Organization"}`}}
	r := f.start(t)
	f.hnAbout = "we build https://github.com/acme"
	p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if _, ok := p.Best(KindGitHub); ok {
		t.Fatal("an organisation account is not the person's GitHub")
	}
}

type memCache struct{ m map[string][]byte }

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool)             { v, ok := c.m[k]; return v, ok }
func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) { c.m[k] = v }

func TestFailedSourceIsNotCached(t *testing.T) {
	f := &fakeWeb{}
	r := f.start(t)
	setURL(t, &hnUserURL, f.srv.URL+"/hn/broken?%s")
	cache := &memCache{m: map[string][]byte{}}
	r.cache = cache
	p, _ := r.Resolve(context.Background(), Seed{Platform: "hackernews", Handle: "janed"})
	if !p.Incomplete || len(cache.m) != 0 {
		t.Fatalf("an outage must not be cached as the answer: incomplete=%v cached=%d", p.Incomplete, len(cache.m))
	}
}

func TestExtractLinksBareDomains(t *testing.T) {
	got := map[string]bool{}
	for _, c := range ExtractLinks("I write at jane.dev and lucabytheway.com/about. Uses node.js, see file.txt or mail jane@acme.io") {
		got[c.URL] = true
	}
	for _, want := range []string{"https://jane.dev", "https://lucabytheway.com", "mailto:jane@acme.io"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, not := range []string{"https://node.js", "https://file.txt", "https://acme.io"} {
		if got[not] {
			t.Errorf("%s must not be taken as a site", not)
		}
	}
}
