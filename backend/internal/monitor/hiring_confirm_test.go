package monitor

import (
	"strings"
	"testing"

	"leadecho/internal/enrich"
)

// Cases are the real outcomes of live runs (2026-11-11).
func TestConfirmContact(t *testing.T) {
	person := foundPerson{Name: "Marty Frenzel", Title: "VP of Engineering", Company: "Vista Higher Learning", ProfileURL: "https://linkedin.com/in/martyfrenzel"}
	vista := hiringCompany{size: 210, domain: "vistahigherlearning.com"}
	ok := func(mut func(*enrich.Result)) *enrich.Result {
		r := &enrich.Result{FullName: "Marty Frenzel", Company: "Vista Higher Learning", CompanyDomain: "vistahigherlearning.com", Email: "m@vhl.example", EmailStatus: "verified"}
		if mut != nil {
			mut(r)
		}
		return r
	}

	cases := []struct {
		name      string
		company   string
		co        hiringCompany
		max       int
		person    foundPerson
		contact   *enrich.Result
		confirmed bool
		tooBig    bool
		reasonHas string
	}{
		{"confirmed", "Vista Higher Learning", vista, 5000, person, ok(nil), true, false, ""},
		{"catch-all email still counts", "Vista Higher Learning", vista, 5000, person, ok(func(r *enrich.Result) { r.EmailStatus = "catchall" }), true, false, ""},
		{"no size ceiling: unknown size is fine", "Vista Higher Learning", hiringCompany{}, 0, person, ok(nil), true, false, ""},
		{"size known only from the contact lookup", "Vista Higher Learning", hiringCompany{domain: "vistahigherlearning.com"}, 5000, person, ok(func(r *enrich.Result) { r.CompanySize = 210 }), true, false, ""},
		{"nobody found", "Two Barrels LLC", hiringCompany{size: 3}, 5000, foundPerson{}, nil, false, false, "no decision-maker"},
		{"lookup failed", "TogetherWork", hiringCompany{size: 278}, 5000, person, nil, false, false, "could not look up"},
		{"profile names another employer", "Stock: The Logistics of Inventory Donation", hiringCompany{size: 60, domain: "stock.example"}, 5000, person,
			&enrich.Result{FullName: "F R", Company: "The Conversation", CompanyDomain: "theconversation.com", Email: "f@x.example"}, false, false, "lists The Conversation"},
		// Same name, different company: only the domain tells them apart.
		{"namesake company", "Prophetic", hiringCompany{size: 40, domain: "propheticsoftware.com"}, 5000, person,
			&enrich.Result{FullName: "C B", Company: "Prophetic", CompanyDomain: "prophetic.health", CompanySize: 40, Email: "c@x.example"}, false, false, "lists Prophetic"},
		{"profile names no employer", "Prophetic", hiringCompany{size: 40}, 5000, person, &enrich.Result{FullName: "C B", Email: "c@x.example"}, false, false, "no current employer"},
		{"no domain on either side: name decides", "LVT (LiveView Technologies)", hiringCompany{size: 900}, 5000, person,
			&enrich.Result{FullName: "A", Company: "LVT", Email: "a@x.example"}, true, false, ""},
		{"size unknown", "Choice Hotels International", hiringCompany{domain: "choicehotels.com"}, 5000, person,
			&enrich.Result{FullName: "T P", Company: "Choice Hotels International", CompanyDomain: "choicehotels.com", Email: "t@x.example"}, false, false, "company size unknown"},
		{"too big", "Stripe", hiringCompany{size: 10210, domain: "stripe.com"}, 5000, person,
			&enrich.Result{FullName: "T L", Company: "Stripe", CompanyDomain: "stripe.com", Email: "t@x.example"}, false, true, ""},
		{"no email", "Vista Higher Learning", vista, 5000, person, ok(func(r *enrich.Result) { r.Email = "" }), false, false, "no working email"},
		{"invalid email", "Vista Higher Learning", vista, 5000, person, ok(func(r *enrich.Result) { r.EmailStatus = "invalid" }), false, false, "no working email"},
	}
	for _, c := range cases {
		got := confirmContact(c.company, c.co, c.max, c.person, c.contact)
		if got.confirmed != c.confirmed || got.tooBig != c.tooBig {
			t.Errorf("%s: got %+v", c.name, got)
		}
		if c.reasonHas != "" && !strings.Contains(got.reason, c.reasonHas) {
			t.Errorf("%s: reason %q should mention %q", c.name, got.reason, c.reasonHas)
		}
		if got.confirmed && got.reason != "" {
			t.Errorf("%s: a confirmed contact has no reason, got %q", c.name, got.reason)
		}
	}
}

func TestSameCompany(t *testing.T) {
	same := [][2]string{
		{"LVT (LiveView Technologies)", "LVT"}, {"Vista Higher Learning", "vista higher learning"},
		{"Acme Inc.", "Acme"}, {"Two Barrels LLC", "Two Barrels"},
	}
	for _, p := range same {
		if !sameCompany(p[0], p[1]) {
			t.Errorf("%q and %q are the same employer", p[0], p[1])
		}
	}
	different := [][2]string{
		{"Stock", "The Conversation"}, {"Acme", "Acme Health"}, {"JPMorganChase", "JPMorganChase Corporate Challenge"},
		{"Meta", "Metabase"}, {"Acme", ""}, {"", ""},
	}
	for _, p := range different {
		if sameCompany(p[0], p[1]) {
			t.Errorf("%q and %q must not match", p[0], p[1])
		}
	}
}

func TestCompanyInfoParsing(t *testing.T) {
	got := parseCompanyInfo([]byte(`{"output":{"name":"Acme Inc","domain":"https://www.Acme.io/about","employees":"51-200"}}`), "Acme")
	if got.size != 51 || got.domain != "acme.io" {
		t.Fatalf("got %+v", got)
	}
	if got := parseCompanyInfo([]byte(`{"name":"Stripe","employees":10210,"domain":"stripe.com"}`), "Stripe"); got.size != 10210 || got.domain != "stripe.com" {
		t.Fatalf("unwrapped: got %+v", got)
	}
	// Captured live: asked for JPMorganChase, a provider answered with another company.
	wrong := `{"output":{"name":"JPMorganChase Corporate Challenge","domain":"jpmorganchasecc.com","employees":1}}`
	if got := parseCompanyInfo([]byte(wrong), "JPMorganChase"); got != (hiringCompany{}) {
		t.Fatalf("an answer about another company must be dropped, got %+v", got)
	}
	if got := parseCompanyInfo([]byte(`not json`), "Acme"); got != (hiringCompany{}) {
		t.Fatalf("bad json: got %+v", got)
	}
	for in, want := range map[string]string{"WWW.Acme.io": "acme.io", "http://acme.io?x=1": "acme.io", " acme.io ": "acme.io", "": ""} {
		if got := cleanDomain(in); got != want {
			t.Errorf("cleanDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
