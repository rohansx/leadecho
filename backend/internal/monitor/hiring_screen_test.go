package monitor

import (
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

func TestScreenedJobsKeepsOnlyWhatTheScreenPassed(t *testing.T) {
	jobs := []jobPost{{Company: "Acme"}, {Company: "BairesDev"}, {Company: "Toku"}}
	kept, dropped := screenedJobs(jobs, []ai.ProspectFit{{Index: 2}, {Index: 0}})
	if len(kept) != 2 || kept[0].Company != "Acme" || kept[1].Company != "Toku" {
		t.Fatalf("kept = %+v", kept)
	}
	if len(dropped) != 1 || dropped[0].Company != "BairesDev" {
		t.Fatalf("dropped = %+v", dropped)
	}
}

// An unconfirmed signal has no contact, email or draft: it is kept as a
// record (so the company is not paid for again) but out of the inbox.
func TestOnlyConfirmedHiringSignalsReachTheInbox(t *testing.T) {
	if got := hiringSignalStatus(contactCheck{confirmed: true}); got != database.MentionStatusNew {
		t.Fatalf("confirmed = %q", got)
	}
	if got := hiringSignalStatus(contactCheck{reason: "company size unknown"}); got != database.MentionStatusArchived {
		t.Fatalf("unconfirmed = %q", got)
	}
}

func TestResearchCanRuleALeadOut(t *testing.T) {
	ok := contactCheck{confirmed: true}
	if got := afterResearch(ok, nil); !got.confirmed {
		t.Fatal("no research: the lead stands")
	}
	if got := afterResearch(ok, &ai.ResearchBrief{WhatTheyDo: "Makes mugs."}); !got.confirmed {
		t.Fatal("research without a verdict: the lead stands")
	}
	got := afterResearch(ok, &ai.ResearchBrief{NotABuyer: "It is a staffing firm."})
	if got.confirmed || got.reason == "" {
		t.Fatalf("ruled out = %+v", got)
	}
}
