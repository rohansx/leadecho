package monitor

import (
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
