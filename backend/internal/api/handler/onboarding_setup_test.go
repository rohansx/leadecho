package handler

import (
	"testing"
	"time"

	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

var setupT0 = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

func TestBuildSetupStatus(t *testing.T) {
	counts := func(posts, unscored, postLeads, companyLeads int) database.SetupCounts {
		return database.SetupCounts{Posts: posts, Unscored: unscored, PostLeads: postLeads, CompanyLeads: companyLeads}
	}
	nextPaid, nextCompany := setupT0.Add(20*time.Hour), setupT0.Add(23*time.Hour)
	for name, tc := range map[string]struct {
		f      setupFacts
		states [4]string // posts, scoring, paid_sources, company_leads
		check  func(t *testing.T, s setupStatus)
	}{
		"free plan, the round has not reached it": {
			f:      setupFacts{Searches: 6, PaidLocked: true, CompanyEnabled: true},
			states: [4]string{stageWaiting, stageWaiting, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[0].EveryMinutes != 5 || s.Stages[2].Reason != "plan" || s.Stages[3].Reason != "plan" || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"free plan, searching and scoring": {
			f:      setupFacts{Counts: counts(40, 15, 0, 0), Searches: 6, Searched: 2, PaidLocked: true, CompanyEnabled: true},
			states: [4]string{stageRunning, stageRunning, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if *s.Stages[0].Posts != 40 || *s.Stages[1].Scored != 25 || *s.Stages[1].Unscored != 15 || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"free plan, searched, still scoring": {
			f:      setupFacts{Counts: counts(40, 3, 0, 0), Searches: 6, Searched: 6, PaidLocked: true},
			states: [4]string{stageDone, stageRunning, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.FinishedEmpty || s.Stages[3].Reason != "server" {
					t.Errorf("%+v", s)
				}
			},
		},
		"free plan, finished with nothing": {
			f:      setupFacts{Counts: counts(40, 0, 0, 0), Searches: 6, Searched: 6, PaidLocked: true, CompanyEnabled: true},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if !s.FinishedEmpty || s.HasLead {
					t.Errorf("%+v", s)
				}
			},
		},
		"finished with a lead is not empty": {
			f:      setupFacts{Counts: counts(40, 0, 2, 0), Searches: 6, Searched: 6, PaidLocked: true},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.FinishedEmpty || !s.HasLead || s.Leads.Post != 2 {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, scan asked for and not started": {
			f:      setupFacts{Searches: 9, PaidSearches: 3, Queued: true, CompanyEnabled: true, CompanyScan: monitor.CompanyScanNever},
			states: [4]string{stageWaiting, stageWaiting, stageWaiting, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[0].Reason != "queued" || s.Stages[2].Reason != "queued" || s.Stages[3].Reason != "queued" || s.Stages[3].EveryMinutes != 0 {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, free sources searched, paid ones and the company scan wait for the next round": {
			f:      setupFacts{Counts: counts(30, 0, 0, 0), Searches: 9, Searched: 6, PaidSearches: 3, CompanyEnabled: true, CompanyScan: monitor.CompanyScanNever},
			states: [4]string{stageRunning, stageWaiting, stageWaiting, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[2].EveryMinutes != 5 || s.Stages[2].NextAt != nil || s.Stages[3].EveryMinutes != 5 || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, company scan running": {
			f:      setupFacts{Counts: counts(30, 0, 0, 0), Searches: 9, Searched: 9, PaidSearches: 3, PaidSearched: 3, PaidNextAt: nextPaid, CompanyEnabled: true, CompanyScan: monitor.CompanyScanRunning},
			states: [4]string{stageDone, stageDone, stageDone, stageRunning},
			check: func(t *testing.T, s setupStatus) {
				if !s.Stages[2].NextAt.Equal(nextPaid) || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, everything ran, a company lead": {
			f: setupFacts{Counts: counts(30, 0, 0, 1), Searches: 9, Searched: 9, PaidSearches: 3, PaidSearched: 3, PaidNextAt: nextPaid,
				CompanyEnabled: true, CompanyScan: monitor.CompanyScanDone, CompanyNextDue: nextCompany},
			states: [4]string{stageDone, stageDone, stageDone, stageDone},
			check: func(t *testing.T, s setupStatus) {
				if !s.Stages[3].NextAt.Equal(nextCompany) || s.FinishedEmpty || !s.HasLead || s.Leads.Company != 1 {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, everything ran, nothing found": {
			f: setupFacts{Counts: counts(30, 0, 0, 0), Searches: 9, Searched: 9, PaidSearches: 3, PaidSearched: 3, PaidNextAt: nextPaid,
				CompanyEnabled: true, CompanyScan: monitor.CompanyScanDone, CompanyNextDue: nextCompany, EmailWhenReady: true},
			states: [4]string{stageDone, stageDone, stageDone, stageDone},
			check: func(t *testing.T, s setupStatus) {
				if !s.FinishedEmpty || !s.EmailWhenReady {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid plan, no keyword on a paid source, product with no hiring signal": {
			f:      setupFacts{Counts: counts(30, 0, 0, 0), Searches: 6, Searched: 6, CompanyEnabled: true, CompanyScan: monitor.CompanyScanNoFit},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[2].Reason != "not_selected" || s.Stages[3].Reason != "no_fit" || !s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"out of credits: posts wait unscored, nothing is called finished": {
			f:      setupFacts{Counts: counts(30, 30, 0, 0), Searches: 6, Searched: 6, PaidLocked: true, Blocked: "out_of_credits"},
			states: [4]string{stageDone, stageWaiting, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[1].Reason != "out_of_credits" || s.Blocked != "out_of_credits" || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid data paused for everyone: paid searches recorded as run were not made": {
			f: setupFacts{Counts: counts(30, 0, 0, 0), Searches: 9, Searched: 9, PaidSearches: 3, PaidSearched: 3, PaidNextAt: nextPaid, PaidPaused: true,
				CompanyEnabled: true, CompanyScan: monitor.CompanyScanNever},
			states: [4]string{stageDone, stageDone, stageWaiting, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[2].Reason != "paid_paused" || s.Stages[2].NextAt != nil || s.Stages[2].EveryMinutes != 0 || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"the company scan failed and is tried again: not finished": {
			f: setupFacts{Counts: counts(30, 0, 0, 0), Searches: 9, Searched: 9, PaidSearches: 3, PaidSearched: 3, PaidNextAt: nextPaid,
				CompanyEnabled: true, CompanyScan: monitor.CompanyScanRetry, CompanyNextDue: nextCompany},
			states: [4]string{stageDone, stageDone, stageDone, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[3].Reason != "retry" || !s.Stages[3].NextAt.Equal(nextCompany) || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"the company scan failed and the workspace is out of credits: no time is promised": {
			f: setupFacts{Counts: counts(30, 0, 0, 0), Searches: 6, Searched: 6, Blocked: "out_of_credits",
				CompanyEnabled: true, CompanyScan: monitor.CompanyScanRetry, CompanyNextDue: nextCompany},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[3].Reason != "out_of_credits" || s.Stages[3].NextAt != nil || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"a lead and unscored posts: not empty, scoring still runs": {
			f:      setupFacts{Counts: counts(40, 5, 1, 0), Searches: 6, Searched: 6, PaidLocked: true},
			states: [4]string{stageDone, stageRunning, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.FinishedEmpty || !s.HasLead {
					t.Errorf("%+v", s)
				}
			},
		},
		"searched and found no post at all: finished empty": {
			f:      setupFacts{Searches: 6, Searched: 6, PaidLocked: true},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if !s.FinishedEmpty || *s.Stages[1].Scored != 0 {
					t.Errorf("%+v", s)
				}
			},
		},
		"paused by an operator mid-search: waiting, not running, not finished": {
			f:      setupFacts{Counts: counts(10, 0, 0, 0), Searches: 6, Searched: 2, PaidLocked: true, Blocked: "paused"},
			states: [4]string{stageWaiting, stageWaiting, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[0].Reason != "paused" || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"paid data paused for everyone: the company scan waits": {
			f:      setupFacts{Searches: 6, Searched: 6, PaidPaused: true, CompanyEnabled: true, CompanyScan: monitor.CompanyScanNever},
			states: [4]string{stageDone, stageDone, stageNotIncluded, stageWaiting},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[3].Reason != "paused" || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
		"this process is scanning the workspace": {
			f:      setupFacts{Searches: 6, Scanning: true, PaidLocked: true},
			states: [4]string{stageRunning, stageWaiting, stageNotIncluded, stageNotIncluded},
		},
		"no active keyword": {
			f:      setupFacts{PaidLocked: true},
			states: [4]string{stageWaiting, stageWaiting, stageNotIncluded, stageNotIncluded},
			check: func(t *testing.T, s setupStatus) {
				if s.Stages[0].Reason != "no_keywords" || s.FinishedEmpty {
					t.Errorf("%+v", s)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := buildSetupStatus(tc.f)
			for i, key := range []string{"posts", "scoring", "paid_sources", "company_leads"} {
				if s.Stages[i].Key != key || s.Stages[i].State != tc.states[i] {
					t.Errorf("stage %d = %s/%s, want %s/%s", i, s.Stages[i].Key, s.Stages[i].State, key, tc.states[i])
				}
			}
			if tc.check != nil {
				tc.check(t, s)
			}
		})
	}
}

func TestSearchProgress(t *testing.T) {
	kws := []database.ListKeywordCadencesRow{
		{ID: "a", Platforms: []string{"hackernews", "reddit", "twitter"}, IsActive: true, Cadence: []byte(`{"twitter":"6h"}`)},
		{ID: "b", Platforms: []string{"devto", "linkedin"}, IsActive: true},
		{ID: "c", Platforms: []string{"hackernews"}, IsActive: false}, // past the plan's keyword limit
	}
	scans := []database.ListKeywordPlatformScansRow{
		{KeywordID: "a", Platform: "hackernews", LastScannedAt: setupT0},
		{KeywordID: "a", Platform: "reddit", LastScannedAt: setupT0},                     // default for a paid source: once a day
		{KeywordID: "a", Platform: "twitter", LastScannedAt: setupT0.Add(-time.Hour)},    // every 6h: next in 5h
		{KeywordID: "c", Platform: "hackernews", LastScannedAt: setupT0.Add(-time.Hour)}, // inactive: not counted
	}
	searches, searched, paid, paidSearched, next := searchProgress(kws, scans, false)
	if searches != 5 || searched != 3 || paid != 3 || paidSearched != 2 || !next.Equal(setupT0.Add(5*time.Hour)) {
		t.Fatalf("paid plan: %d %d %d %d %v", searches, searched, paid, paidSearched, next)
	}
	// Without paid data the paid platforms are not searched at all.
	searches, searched, paid, paidSearched, next = searchProgress(kws, scans, true)
	if searches != 2 || searched != 1 || paid != 0 || paidSearched != 0 || !next.IsZero() {
		t.Fatalf("free plan: %d %d %d %d %v", searches, searched, paid, paidSearched, next)
	}
}

// The legacy flat complete request carries no archetype: it must not wipe
// the stored one (NULL keeps it in the upsert).
func TestLegacyCompleteKeepsArchetype(t *testing.T) {
	p := completeRequest{ProductName: "Acme", Description: "CRM"}.brandProfile().Params("ws")
	if p.Archetype.Valid {
		t.Errorf("legacy complete would overwrite the archetype: %+v", p.Archetype)
	}
}
