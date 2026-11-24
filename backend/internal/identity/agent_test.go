package identity

import (
	"testing"

	"leadecho/internal/ai"
)

func TestMergeAgentKeepsOnlyWhatPagesShow(t *testing.T) {
	base := &Profile{Tier: "a", Name: "Jane Doe", Links: []Link{
		{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=janed", Handle: "janed", Confidence: 1, Method: "origin"},
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
	if len(base.Links) != 2 || base.Tier != "a" {
		t.Error("MergeAgent must not modify its input")
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
	// The HN profile links to jane.dev, and jane.dev links to GitHub (trusted
	// only once jane.dev is). A Mastodon profile that links to jane.dev, with
	// nothing of theirs linking back, could be anyone who links to that site:
	// a candidate. A third-party page's email and LinkedIn are not theirs.
	base := &Profile{Links: []Link{{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=janed", Handle: "janed", Confidence: 1, Method: "origin"}}}
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
	for _, kind := range []string{KindWebsite, KindGitHub} {
		if _, ok := p.Best(kind); !ok {
			t.Errorf("%s should be shown: %+v", kind, p.Links)
		}
	}
	if c := linkConf(p, "https://hachyderm.io/@jane"); c >= Shown {
		t.Errorf("a one-way inbound link must not be trusted, conf %.2f", c)
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
		Name: "Glen Baker",
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

func TestMergeAgentEmployerSiteAndRoleMailboxes(t *testing.T) {
	// Their GitHub lists the employer's site. The agent reads acme.com/team
	// and claims a colleague's LinkedIn and email, plus support@ on their
	// own site: none of it is theirs.
	base := &Profile{Name: "Jane Doe", Links: []Link{
		{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=janed", Handle: "janed", Confidence: 1, Method: "origin"},
		{Kind: KindGitHub, URL: "https://github.com/janed", Handle: "janed", Confidence: 0.95},
		{Kind: KindWebsite, URL: "https://acme.com", Confidence: 0.92},
		{Kind: KindWebsite, URL: "https://jane.dev", Confidence: 0.95},
	}}
	f := &ai.PersonFindings{
		Seen: map[string]string{
			"https://acme.com/team": "https://www.linkedin.com/in/bob-smith bob@acme.com jane@acme.com",
			"https://jane.dev/":     "support@jane.dev",
		},
		Links: []ai.AgentLink{
			{URL: "https://www.linkedin.com/in/bob-smith", Evidence: "https://acme.com/team"},
			{URL: "mailto:bob@acme.com", Evidence: "https://acme.com/team"},
			{URL: "mailto:jane@acme.com", Evidence: "https://acme.com/team"},
			{URL: "mailto:support@jane.dev", Evidence: "https://jane.dev/"},
		},
	}
	p := MergeAgent(base, f)
	if _, ok := p.Best(KindLinkedIn); ok {
		t.Error("a colleague's LinkedIn on the employer's team page is not theirs")
	}
	if p.Email() != "" {
		// jane@acme.com names only a first name: on a team page that is not
		// enough (the paid lookup can still find their work email).
		t.Errorf("no address on the employer's page names them, got %q", p.Email())
	}
}

func TestColleagueWithTheSameFirstNameIsNotThem(t *testing.T) {
	base := &Profile{Name: "Jane Doe", Links: []Link{
		{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=jd", Handle: "jd", Confidence: 1, Method: "origin"},
		{Kind: KindWebsite, URL: "https://acme.com", Confidence: 0.92},
	}}
	f := &ai.PersonFindings{
		Seen: map[string]string{"https://acme.com/team": "https://www.linkedin.com/in/jane-cooper jane.cooper@acme.com https://www.linkedin.com/in/jane-doe-1"},
		Links: []ai.AgentLink{
			{URL: "https://www.linkedin.com/in/jane-cooper", Evidence: "https://acme.com/team"},
			{URL: "mailto:jane.cooper@acme.com", Evidence: "https://acme.com/team"},
			{URL: "https://www.linkedin.com/in/jane-doe-1", Evidence: "https://acme.com/team"},
		},
	}
	p := MergeAgent(base, f)
	if c := linkConf(p, "https://www.linkedin.com/in/jane-cooper"); c >= Shown {
		t.Errorf("same first name is not the same person, conf %.2f", c)
	}
	if p.Email() != "" {
		t.Errorf("a colleague's email must not count, got %q", p.Email())
	}
	if _, ok := p.Best(KindLinkedIn); !ok {
		t.Error("their surname in the slug should count (jane-doe-1)")
	}
}

func TestWorksKeepOnlyWhatIsProvablyTheirs(t *testing.T) {
	base := &Profile{Name: "Glen Baker", Links: []Link{
		{Kind: KindHackerNews, URL: "https://news.ycombinator.com/user?id=iepathos", Handle: "iepathos", Confidence: 1, Method: "origin"},
		{Kind: KindWebsite, URL: "https://entropicdrift.com", Confidence: 0.9},
		{Kind: KindGitHub, URL: "https://github.com/iepathos", Handle: "iepathos", Confidence: 0.9},
	}}
	f := &ai.PersonFindings{
		Seen: map[string]string{
			"https://confs.example/2026/speakers": "Talk: Paying down tech debt, by Glen Baker",
			"https://news.example/rust-tools":     "A roundup mentioning Debtmap, made by some developer",
		},
		Works: []ai.Work{
			{Kind: "blog", Title: "Debt is a design smell", URL: "https://entropicdrift.com/posts/debt", Date: "2026-10"}, // their site
			{Kind: "repo", Title: "debtmap", URL: "https://github.com/iepathos/debtmap", Date: ""},                        // their repo
			{Kind: "article", Title: "Rust tips", URL: "https://dev.to/iepathos/rust-tips", Date: "2026-08"},              // their username path
			{Kind: "talk", Title: "Paying down tech debt", URL: "https://confs.example/2026/speakers", Date: "2026-05"},   // page names them
			{Kind: "article", Title: "Rust tool roundup", URL: "https://news.example/rust-tools", Date: "2026-09"},        // about their work, not by them
			{Kind: "repo", Title: "someone else's fork", URL: "https://github.com/otherdev/debtmap", Date: ""},            // not theirs
			{Kind: "blog", Title: "bad", URL: "javascript:alert(1)", Date: ""},                                            // not a link
		},
	}
	p := MergeAgent(base, f)
	got := map[string]bool{}
	for _, w := range p.Works {
		got[w.URL] = true
	}
	for _, want := range []string{"https://entropicdrift.com/posts/debt", "https://github.com/iepathos/debtmap",
		"https://dev.to/iepathos/rust-tips", "https://confs.example/2026/speakers"} {
		if !got[want] {
			t.Errorf("missing their work %s", want)
		}
	}
	for _, not := range []string{"https://news.example/rust-tools", "https://github.com/otherdev/debtmap", "javascript:alert(1)"} {
		if got[not] {
			t.Errorf("%s is not provably theirs", not)
		}
	}
	if p.Works[0].Date != "2026-10" && p.Works[0].URL != "https://entropicdrift.com/posts/debt" {
		t.Errorf("most recent first, got %+v", p.Works[0])
	}
}

func TestMergeWorksKeepsOldAndDedupes(t *testing.T) {
	old := []Work{{Title: "a", URL: "https://jane.dev/a"}}
	got := mergeWorks(old, []Work{{Title: "a again", URL: "https://jane.dev/a/"}, {Title: "b", URL: "https://jane.dev/b"}})
	if len(got) != 2 || got[0].Title != "a" || got[1].Title != "b" {
		t.Fatalf("got %+v", got)
	}
	if len(old) != 1 {
		t.Fatal("must not modify its input")
	}
}
