package monitor

import (
	"strings"
	"testing"

	"leadecho/internal/enrich"
)

func TestBuyerQuery(t *testing.T) {
	job := jobPost{Company: "Affirm", Title: "Software Engineer I, Fullstack (Search)"}
	titles := "CTO, VP Engineering or Head of Engineering"
	// Small or unknown size: the buyer titles are the people hiring.
	for _, size := range []int{0, 210, hiringTeamContactSize - 1} {
		if got := buyerQuery(titles, job, size); got != titles+" at Affirm" {
			t.Errorf("size %d: got %q", size, got)
		}
	}
	// Larger company: ask for whoever is hiring for this role.
	want := "manager or recruiter hiring for Software Engineer I, Fullstack (Search) at Affirm"
	if got := buyerQuery(titles, job, 1555); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// No job title to name: fall back to the titles.
	if got := buyerQuery(titles, jobPost{Company: "Affirm"}, 1555); got != titles+" at Affirm" {
		t.Errorf("no job title: got %q", got)
	}
}

func TestPickBuyersReturnsCandidatesInOrder(t *testing.T) {
	people := []foundPerson{
		{Name: "Former", Title: "CTO", Company: "Other Co", ProfileURL: "https://linkedin.com/in/former"},
		{Name: "First", Title: "Senior Technical Recruiter", Company: "Affirm", ProfileURL: "https://linkedin.com/in/first"},
		{Name: "IC", Title: "Software Engineer", Company: "Affirm", ProfileURL: "https://linkedin.com/in/ic"},
		{Name: "Second", Title: "Engineering Manager", Company: "Affirm", ProfileURL: "https://linkedin.com/in/second"},
		{Name: "Third", Title: "Director, Global Technical Recruiting", Company: "Affirm", ProfileURL: "https://linkedin.com/in/third"},
	}
	got := pickBuyers(people, "Affirm", maxContactTries)
	if len(got) != 2 || got[0].Name != "First" || got[1].Name != "Second" {
		t.Fatalf("want First then Second, got %+v", got)
	}
	if one, ok := pickBuyer(people, "Affirm"); !ok || one.Name != "First" {
		t.Fatalf("pickBuyer should still return the first: %+v", one)
	}
	if got := pickBuyers(people, "Nobody Inc", 2); len(got) != 0 {
		t.Fatalf("no match expected, got %+v", got)
	}
}

func TestHiringLeadScore(t *testing.T) {
	if hiringLeadScore(&enrich.Result{EmailStatus: "verified"}) != hiringScoreConfirmed {
		t.Error("a verified email gets the full score")
	}
	if hiringLeadScore(&enrich.Result{EmailStatus: "catchall"}) != hiringScoreUnverifiedEmail {
		t.Error("a catch-all email ranks below a verified one")
	}
	if hiringScoreUnverifiedEmail < leadScoreThreshold || hiringScoreUnverifiedEmail >= hiringScoreConfirmed {
		t.Error("an unverified-email lead must still be a lead, ranked under a verified one")
	}
	if hiringScoreUnconfirmed >= leadScoreThreshold {
		t.Error("an unconfirmed signal must stay below the lead threshold")
	}
}

func TestChooseContact(t *testing.T) {
	a := foundPerson{Name: "A"}
	b := foundPerson{Name: "B"}
	okRes := &enrich.Result{FullName: "ok"}
	type step struct {
		capped bool
		check  contactCheck
	}
	run := func(cands []foundPerson, steps map[string]step) (foundPerson, contactCheck, bool, []string) {
		var verified []string
		p, _, ck, capped := chooseContact(cands,
			func(c foundPerson) (*enrich.Result, bool) {
				verified = append(verified, c.Name)
				return okRes, steps[c.Name].capped
			},
			func(c foundPerson, _ *enrich.Result) contactCheck {
				if c.Name == "" {
					return contactCheck{reason: "nobody"}
				}
				return steps[c.Name].check
			})
		return p, ck, capped, verified
	}

	// No candidates: the "nobody found" reason, no lookups.
	if p, ck, capped, v := run(nil, nil); p.Name != "" || ck.reason != "nobody" || capped || len(v) != 0 {
		t.Fatalf("empty: %+v %+v %v %v", p, ck, capped, v)
	}
	// First confirmed: the second is never paid for.
	if p, ck, _, v := run([]foundPerson{a, b}, map[string]step{"A": {check: contactCheck{confirmed: true}}}); p.Name != "A" || !ck.confirmed || len(v) != 1 {
		t.Fatalf("first confirmed: %+v %+v %v", p, ck, v)
	}
	// First unconfirmed, second confirmed: everything is the second's.
	if p, ck, _, v := run([]foundPerson{a, b}, map[string]step{"A": {check: contactCheck{reason: "moved"}}, "B": {check: contactCheck{confirmed: true}}}); p.Name != "B" || !ck.confirmed || len(v) != 2 {
		t.Fatalf("second confirmed: %+v %+v %v", p, ck, v)
	}
	// Neither confirms: the first one's result is kept.
	if p, ck, _, _ := run([]foundPerson{a, b}, map[string]step{"A": {check: contactCheck{reason: "moved"}}, "B": {check: contactCheck{reason: "no email"}}}); p.Name != "A" || ck.reason != "moved" {
		t.Fatalf("neither: %+v %+v", p, ck)
	}
	// Too big on the first stops immediately.
	if p, ck, _, v := run([]foundPerson{a, b}, map[string]step{"A": {check: contactCheck{tooBig: true}}}); p.Name != "A" || !ck.tooBig || len(v) != 1 {
		t.Fatalf("too big: %+v %+v %v", p, ck, v)
	}
	// Cap before anyone was checked: nothing to store.
	if p, _, capped, _ := run([]foundPerson{a, b}, map[string]step{"A": {capped: true}}); !capped || p.Name != "" {
		t.Fatalf("cap on first: %+v %v", p, capped)
	}
	// Cap on the second: the first one's paid-for result is kept so it can be stored.
	if p, ck, capped, _ := run([]foundPerson{a, b}, map[string]step{"A": {check: contactCheck{reason: "moved"}}, "B": {capped: true}}); !capped || p.Name != "A" || ck.reason != "moved" {
		t.Fatalf("cap on second: %+v %+v %v", p, ck, capped)
	}
}

func TestBuyerQueryCapsLongTitles(t *testing.T) {
	long := strings.Repeat("é", 400)
	got := buyerQuery("CTO", jobPost{Company: "Acme", Title: long}, 900)
	if n := len([]rune(got)); n > 120+len([]rune("manager or recruiter hiring for  at Acme")) {
		t.Fatalf("query not capped: %d runes", n)
	}
}
