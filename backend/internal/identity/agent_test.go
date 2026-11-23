package identity

import (
	"testing"

	"leadecho/internal/ai"
)

func TestMergeAgentKeepsOnlyWhatPagesShow(t *testing.T) {
	base := &Profile{Tier: "a", Links: []Link{
		{Kind: KindWebsite, URL: "https://jane.dev", Confidence: 0.95, Method: "bio_link"},
	}}
	f := &ai.PersonFindings{
		Name: "Jane Doe", Role: "CTO", Company: "Acme", Summary: "Builds data tools.", Topics: []string{"databases"},
		Seen: map[string]string{
			"https://jane.dev/about": `Jane Doe. Find me on https://www.linkedin.com/in/jane-doe and github.com/janed. Email: jane at jane dot dev`,
			"https://other.example":  `nothing here`,
		},
		SearchURLs: map[string]bool{"https://www.linkedin.com/in/jane-doe-2": true},
		Links: []ai.AgentLink{
			{Kind: "linkedin", URL: "https://www.linkedin.com/in/jane-doe", Evidence: "https://jane.dev/about"}, // on a page it read
			{Kind: "github", URL: "https://github.com/janed", Evidence: "https://jane.dev/about"},               // on a page it read
			{Kind: "email", URL: "mailto:jane@jane.dev", Evidence: "https://jane.dev/about"},                    // spelled out on their own site
			{Kind: "x", URL: "https://x.com/made_up", Evidence: "https://other.example"},                        // not on that page
			{Kind: "email", URL: "mailto:jane@acme.com", Evidence: "https://other.example"},                     // a guessed pattern
			{Kind: "linkedin", URL: "https://www.linkedin.com/in/jane-doe-2", Evidence: "web search"},           // search result only
			{Kind: "website", URL: "javascript:alert(1)", Evidence: "https://jane.dev/about"},                   // not a link
		},
	}
	p := MergeAgent(base, f)

	if l, ok := p.Best(KindLinkedIn); !ok || l.URL != "https://www.linkedin.com/in/jane-doe" {
		t.Errorf("LinkedIn seen on a read page should be shown, got %+v", p.Links)
	}
	if _, ok := p.Best(KindGitHub); !ok {
		t.Error("GitHub seen on a read page should be shown")
	}
	if p.Email() != "jane@jane.dev" {
		t.Errorf("spelled-out email on their own site should be kept, got %q", p.Email())
	}
	if c := linkConf(p, "https://x.com/made_up"); c != 0 {
		t.Errorf("a link not on its evidence page must be dropped, conf %.2f", c)
	}
	if c := linkConf(p, "mailto:jane@acme.com"); c != 0 {
		t.Errorf("a guessed email must be dropped, conf %.2f", c)
	}
	if c := linkConf(p, "https://www.linkedin.com/in/jane-doe-2"); c == 0 || c >= Shown {
		t.Errorf("a search-only LinkedIn is a candidate below Shown, conf %.2f", c)
	}
	if p.Tier != "b" || p.Name != "Jane Doe" || p.Role != "CTO" || p.Summary == "" || len(p.Topics) != 1 {
		t.Errorf("fields not merged: %+v", p)
	}
	if len(base.Links) != 1 || base.Tier != "a" {
		t.Error("MergeAgent must not modify its input")
	}
}

func TestConfirmRaisesOneLinkWithoutMutating(t *testing.T) {
	p := &Profile{Links: []Link{{Kind: KindLinkedIn, URL: "https://www.linkedin.com/in/x", Confidence: 0.7}}}
	q := Confirm(p, "https://www.linkedin.com/in/x", 0.9, "name_confirmed")
	if _, ok := q.Best(KindLinkedIn); !ok {
		t.Fatal("confirmed link should be shown")
	}
	if p.Links[0].Confidence != 0.7 {
		t.Fatal("Confirm must not modify its input")
	}
}

func TestMergeAgentNeedsExactLinkNotSubstring(t *testing.T) {
	base := &Profile{Links: []Link{{Kind: KindWebsite, URL: "https://jane.dev", Confidence: 0.95}}}
	f := &ai.PersonFindings{
		Seen: map[string]string{"https://jane.dev/": "see linkedin.com/in/jane-doe-4a2b9 and mail jane.doe@jane.dev"},
		Links: []ai.AgentLink{
			{URL: "https://www.linkedin.com/in/jane-doe", Evidence: "https://jane.dev/"}, // a prefix of the real slug
			{URL: "mailto:jane@jane.dev", Evidence: "https://jane.dev/"},                 // local part guessed from the name
		},
	}
	p := MergeAgent(base, f)
	if c := linkConf(p, "https://www.linkedin.com/in/jane-doe"); c != 0 {
		t.Errorf("a slug that only prefixes the real one must not count, conf %.2f", c)
	}
	if p.Email() != "" {
		t.Errorf("a guessed address must not count, got %q", p.Email())
	}
}

func TestMergeAgentChainsAndLinkBack(t *testing.T) {
	// The HN profile links to jane.dev, jane.dev links to GitHub (trusted only
	// once jane.dev is), and a Mastodon profile the agent opened links back to
	// jane.dev. A third-party page listing an email and a LinkedIn is not theirs.
	base := &Profile{Links: []Link{{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=janed", Confidence: 1, Method: "origin"}}}
	f := &ai.PersonFindings{
		Seen: map[string]string{
			"https://news.ycombinator.com/user?id=janed": "site: https://jane.dev",
			"https://jane.dev/about":                     "code: https://github.com/janed",
			"https://hachyderm.io/@jane":                 "Jane. https://jane.dev",
			"https://spam.example/list":                  "jane@jane.dev https://www.linkedin.com/in/jane-doe",
		},
		Links: []ai.AgentLink{
			{URL: "https://github.com/janed", Evidence: "https://jane.dev/about"}, // needs jane.dev trusted first
			{URL: "https://jane.dev", Evidence: "https://news.ycombinator.com/user?id=janed"},
			{URL: "https://hachyderm.io/@jane", Evidence: "https://hachyderm.io/@jane"}, // opened, links back
			{URL: "mailto:jane@jane.dev", Evidence: "https://spam.example/list"},
			{URL: "https://www.linkedin.com/in/jane-doe", Evidence: "https://spam.example/list"},
		},
	}
	p := MergeAgent(base, f)
	for _, kind := range []string{KindWebsite, KindGitHub, KindMastodon} {
		if _, ok := p.Best(kind); !ok {
			t.Errorf("%s should be shown: %+v", kind, p.Links)
		}
	}
	if p.Email() != "" {
		t.Errorf("an email on a page that isn't theirs must not count, got %q", p.Email())
	}
	if c := linkConf(p, "https://www.linkedin.com/in/jane-doe"); c >= Shown || c == 0 {
		t.Errorf("LinkedIn on a third-party page is a candidate below Shown, conf %.2f", c)
	}
}

func TestMergeAgentSameHandleCluster(t *testing.T) {
	// The HN "about" is empty. GitHub "iepathos" and entropicdrift.com link to
	// each other; the site lists a LinkedIn. One shared username alone stays
	// a candidate; a two-way-linked pair under it is shown.
	origin := Link{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=iepathos", Handle: "iepathos", Confidence: 1, Method: "origin"}
	f := &ai.PersonFindings{
		Seen: map[string]string{
			"https://github.com/iepathos": "Glen Baker https://entropicdrift.com",
			"https://entropicdrift.com/":  "Glen Baker https://github.com/iepathos https://www.linkedin.com/in/glenbbaker",
			"https://x.com/iepathos":      "some other person",
		},
		Links: []ai.AgentLink{
			{URL: "https://entropicdrift.com", Evidence: "https://github.com/iepathos"},
			{URL: "https://github.com/iepathos", Evidence: "https://entropicdrift.com"},
			{URL: "https://www.linkedin.com/in/glenbbaker/", Evidence: "https://entropicdrift.com"},
			{URL: "https://x.com/iepathos", Evidence: "https://x.com/iepathos"},
		},
	}
	p := MergeAgent(&Profile{Links: []Link{origin}}, f)
	for _, kind := range []string{KindGitHub, KindWebsite, KindLinkedIn} {
		if _, ok := p.Best(kind); !ok {
			t.Errorf("%s should be shown: %+v", kind, p.Links)
		}
	}
	if c := linkConf(p, "https://x.com/iepathos"); c >= Shown {
		t.Errorf("same username without a two-way link stays a candidate, conf %.2f", c)
	}

	// A short, common username never anchors a cluster.
	short := Link{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=bob", Handle: "bob", Confidence: 1, Method: "origin"}
	g := &ai.PersonFindings{
		Seen: map[string]string{
			"https://github.com/bob": "https://bob.dev",
			"https://bob.dev/":       "https://github.com/bob",
		},
		Links: []ai.AgentLink{{URL: "https://github.com/bob", Evidence: "https://bob.dev"}},
	}
	if _, ok := MergeAgent(&Profile{Links: []Link{short}}, g).Best(KindGitHub); ok {
		t.Error("a 3-letter username must not anchor a cluster")
	}
}
