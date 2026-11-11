package monitor

import (
	"strings"
	"testing"

	"leadecho/internal/enrich"
)

// Cases are the real outcomes of a live run (2026-11-11).
func TestConfirmContact(t *testing.T) {
	person := foundPerson{Name: "Marty Frenzel", Title: "VP of Engineering", Company: "Vista Higher Learning", ProfileURL: "https://linkedin.com/in/martyfrenzel"}
	good := &enrich.Result{FullName: "Marty Frenzel", Title: "VP of Engineering", Company: "Vista Higher Learning", CompanySize: 210, Email: "m@vhl.example", EmailStatus: "verified"}

	cases := []struct {
		name       string
		company    string
		max, known int
		person     foundPerson
		contact    *enrich.Result
		confirmed  bool
		tooBig     bool
		reasonHas  string
	}{
		{"confirmed", "Vista Higher Learning", 5000, 0, person, good, true, false, ""},
		{"catch-all email still counts", "Vista Higher Learning", 5000, 0, person, &enrich.Result{FullName: "M", Company: "Vista Higher Learning", CompanySize: 210, Email: "m@vhl.example", EmailStatus: "catchall"}, true, false, ""},
		{"no size ceiling: unknown size is fine", "Vista Higher Learning", 0, 0, person, &enrich.Result{FullName: "M", Company: "Vista Higher Learning", Email: "m@vhl.example"}, true, false, ""},
		{"nobody found", "Two Barrels LLC", 5000, 3, foundPerson{}, nil, false, false, "no decision-maker"},
		{"lookup failed", "TogetherWork", 5000, 278, person, nil, false, false, "could not look up"},
		{"profile names another employer", "Stock: The Logistics of Inventory Donation", 5000, 60, person, &enrich.Result{FullName: "F R", Company: "The Conversation", CompanySize: 60, Email: "f@x.example"}, false, false, "lists The Conversation"},
		{"profile names no employer", "Prophetic", 5000, 0, person, &enrich.Result{FullName: "C B", Email: "c@x.example"}, false, false, "no current employer"},
		{"size unknown", "Choice Hotels International", 5000, 0, person, &enrich.Result{FullName: "T P", Company: "Choice Hotels International", Email: "t@x.example", EmailStatus: "verified"}, false, false, "company size unknown"},
		{"size known only from the pre-check", "Acme", 5000, 120, person, &enrich.Result{FullName: "A", Company: "Acme", Email: "a@x.example"}, true, false, ""},
		{"too big after lookup", "Stripe", 5000, 0, person, &enrich.Result{FullName: "T L", Company: "Stripe", CompanySize: 10210, Email: "t@x.example"}, false, true, ""},
		{"no email", "Vista Higher Learning", 5000, 0, person, &enrich.Result{FullName: "M", Company: "Vista Higher Learning", CompanySize: 210}, false, false, "no working email"},
		{"invalid email", "Vista Higher Learning", 5000, 0, person, &enrich.Result{FullName: "M", Company: "Vista Higher Learning", CompanySize: 210, Email: "m@vhl.example", EmailStatus: "invalid"}, false, false, "no working email"},
	}
	for _, c := range cases {
		got := confirmContact(c.company, c.max, c.known, c.person, c.contact)
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
	if !sameCompany("LVT (LiveView Technologies)", "LVT") || !sameCompany("Vista Higher Learning", "vista higher learning") {
		t.Fatal("same employer written differently should match")
	}
	if sameCompany("Stock", "The Conversation") || sameCompany("Acme", "") || sameCompany("", "") {
		t.Fatal("different or missing employers must not match")
	}
}
