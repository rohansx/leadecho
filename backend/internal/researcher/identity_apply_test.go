package researcher

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/enrich"
	"leadecho/internal/identity"
)

func TestWithIdentityAddsOnlyShownKeys(t *testing.T) {
	p := &identity.Profile{Name: "Jane Doe", Company: "Acme", Links: []identity.Link{
		{Kind: identity.KindLinkedIn, URL: "https://www.linkedin.com/in/jane-doe", Confidence: 0.9},
		{Kind: identity.KindEmail, URL: "mailto:jane@jane.dev", Handle: "jane@jane.dev", Confidence: 0.85, Evidence: "https://jane.dev"},
		{Kind: identity.KindLinkedIn, URL: "https://www.linkedin.com/in/someone-else", Confidence: 0.5},
	}}
	got := withIdentity(enrich.Subject{Platform: "hackernews", Handle: "janed"}, p)
	want := enrich.Subject{Platform: "hackernews", Handle: "janed", Name: "Jane Doe", Company: "Acme",
		ProfileURL: "https://www.linkedin.com/in/jane-doe", Email: "jane@jane.dev", EmailSource: "https://jane.dev"}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	// A typed contact and an existing LinkedIn profile URL are kept.
	keep := enrich.Subject{ProfileURL: "https://www.linkedin.com/in/typed", Email: "typed@x.io", Name: "Typed"}
	if got := withIdentity(keep, p); got.ProfileURL != keep.ProfileURL || got.Email != keep.Email || got.Name != "Typed" {
		t.Fatalf("overwrote what the lead already had: %+v", got)
	}
	if got := withIdentity(keep, nil); got != keep {
		t.Fatal("nil profile must not change the subject")
	}
}

func TestSameName(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Jane Doe", "jane doe", true},
		{"Jane Q. Doe", "Jane Doe", true},
		{"Jane Doe", "Jane Smith", false},
		{"Jane", "Jane", false}, // one word can't confirm anyone
		{"", "Jane Doe", false},
	} {
		if got := sameName(c.a, c.b); got != c.want {
			t.Errorf("sameName(%q,%q) = %v", c.a, c.b, got)
		}
	}
}

func TestAgentDue(t *testing.T) {
	complete := &identity.Profile{Tier: "a", Links: []identity.Link{
		{Kind: identity.KindLinkedIn, URL: "https://www.linkedin.com/in/x", Confidence: 0.9},
		{Kind: identity.KindEmail, URL: "mailto:x@x.dev", Handle: "x@x.dev", Confidence: 0.9},
	}}
	if agentDue(complete) {
		t.Error("nothing left to find")
	}
	if !agentDue(&identity.Profile{Tier: "a"}) {
		t.Error("an empty tier-a profile needs the agent")
	}
	if agentDue(&identity.Profile{Tier: "b"}) {
		t.Error("one agent run per author")
	}
	if agentDue(nil) {
		t.Error("no profile (LinkedIn author): treg handles it")
	}
}

func TestDayCounter(t *testing.T) {
	c := &dayCounter{runs: map[string]int{}}
	if !c.take("w", 2) || !c.take("w", 2) || c.take("w", 2) {
		t.Fatal("cap of 2 per workspace per day")
	}
	if !c.take("other", 2) {
		t.Fatal("caps are per workspace")
	}
}

func TestLinkedInConfirmationNeedsASecondSignal(t *testing.T) {
	if !sameCompany("Acme Inc", "acme") || sameCompany("", "acme") || sameCompany("Acme", "Globex") {
		t.Error("sameCompany")
	}
	if !slugHasSurname("jane-doe-4a2b9", "Jane Doe") || slugHasSurname("jsmith", "Jane Doe") || slugHasSurname("jane", "Jane") {
		t.Error("slugHasSurname")
	}
}

func TestFillRulesNeverOverwrite(t *testing.T) {
	set := pgtype.Text{String: "Jane Doe", Valid: true}
	if fillName(set, "J. Doe").Valid {
		t.Error("a real name must not be replaced")
	}
	if got := fillName(pgtype.Text{String: "janed", Valid: true}, "Jane Doe"); got.String != "Jane Doe" {
		t.Error("a handle placeholder should be replaced by the real name")
	}
	if fillIf(pgtype.Text{String: "Acme", Valid: true}, "Other").Valid {
		t.Error("an existing company must not be replaced")
	}
	if fillIf(pgtype.Text{}, "Acme").String != "Acme" {
		t.Error("an empty company should be filled")
	}
}

func TestDayCounterGlobalCap(t *testing.T) {
	c := &dayCounter{runs: map[string]int{}}
	if !c.takeBoth("a", 5, 2) || !c.takeBoth("b", 5, 2) {
		t.Fatal("two runs fit the global cap")
	}
	if c.takeBoth("c", 5, 2) {
		t.Fatal("the global cap holds across workspaces")
	}
	if c.runs["c"] != 0 {
		t.Fatal("a refused run must not be counted")
	}
}

func TestLookupableNeedsAKeyForForumAuthors(t *testing.T) {
	for _, c := range []struct {
		sub  enrich.Subject
		want bool
	}{
		{enrich.Subject{Platform: "hackernews", Handle: "janed"}, false},
		{enrich.Subject{Platform: "hackernews", Handle: "janed", Email: "jane@jane.dev"}, true},
		{enrich.Subject{Platform: "devto", Handle: "janed", ProfileURL: "https://www.linkedin.com/in/jane"}, true},
		{enrich.Subject{Platform: "bluesky", Handle: "jane.dev", ProfileURL: "https://bsky.app/profile/jane.dev"}, false},
		{enrich.Subject{Platform: "linkedin", Handle: "jane"}, true},
		{enrich.Subject{Platform: "twitter", Handle: "jane"}, true},
	} {
		if got := lookupable(c.sub); got != c.want {
			t.Errorf("%+v: got %v, want %v", c.sub, got, c.want)
		}
	}
}

func TestDeepDueOnlyForIdentifiedPeopleOnce(t *testing.T) {
	known := &identity.Profile{Name: "Jane Doe", Tier: "b", Links: []identity.Link{
		{Kind: identity.KindGitHub, URL: "https://github.com/janed", Confidence: 0.9},
	}}
	if !deepDue(known) {
		t.Error("a named, verified person should get one deep run")
	}
	done := *known
	done.DeepAt = time.Now()
	if deepDue(&done) {
		t.Error("one deep run per person")
	}
	if deepDue(&identity.Profile{Name: "Jane", Links: known.Links}) {
		t.Error("a first name alone is not identified")
	}
	if deepDue(&identity.Profile{Name: "Jane Doe"}) {
		t.Error("no verified link: not identified")
	}
	if deepDue(nil) {
		t.Error("nil profile")
	}
}
