package monitor

import (
	"errors"
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// A company lead fails the re-screen when its stored research says the
// company is not a buyer, or when the posting screen no longer keeps its job.
// Prospects have no posting, so only their research counts.
func TestRescreenReasons(t *testing.T) {
	leads := []database.CompanyLead{
		{ID: "keep", Signal: "hiring", Company: "Acme", JobTitle: "Backend Engineer"},
		{ID: "agency", Signal: "hiring", Company: "Staffing Co", JobTitle: "Java Developer"},
		{ID: "ruled-out", Signal: "hiring", Company: "Rival", JobTitle: "Software Engineer", NotABuyer: "It sells a product like this one."},
		{ID: "prospect", Signal: "prospect", Company: "Fits"},
		{ID: "prospect-out", Signal: "prospect", Company: "Agency", NotABuyer: "It hires for its clients."},
		{ID: "no-title", Signal: "hiring", Company: "Old Row"},
	}
	var screened []ai.HiringJob
	got, _, err := rescreenReasons(leads, func(jobs []ai.HiringJob) ([]ai.ProspectFit, error) {
		screened = append(screened, jobs...)
		return []ai.ProspectFit{{Index: 0}}, nil // keeps Acme, drops Staffing Co
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only postings not already ruled out go to the model.
	if len(screened) != 2 || screened[0].Company != "Acme" || screened[1].Company != "Staffing Co" {
		t.Fatalf("screened = %+v, want Acme and Staffing Co", screened)
	}
	if len(got) != 3 || got["agency"] != rescreenPostingReason || got["ruled-out"] == "" || got["prospect-out"] == "" {
		t.Fatalf("reasons = %v, want the agency, and the two companies research ruled out", got)
	}
	// The research's own wording came from web pages: it is not what is stored.
	if got["ruled-out"] == leads[2].NotABuyer {
		t.Fatalf("the stored reason must be the fixed sentence, got the model's: %q", got["ruled-out"])
	}

	// A screen that fails drops nobody.
	if got, _, err := rescreenReasons(leads[:2], func([]ai.HiringJob) ([]ai.ProspectFit, error) {
		return nil, errors.New("model down")
	}); err == nil || got != nil {
		t.Fatalf("a failed screen must fail the run, got %v, %v", got, err)
	}

	// Long lists are screened in batches the model can read, and the indexes
	// of each batch map back to the right leads.
	many := make([]database.CompanyLead, maxHiringScreened+1)
	for i := range many {
		many[i] = database.CompanyLead{ID: string(rune('a'+i%26)) + string(rune('a'+i/26)), Signal: "hiring", JobTitle: "Engineer"}
	}
	calls := 0
	got, _, err = rescreenReasons(many, func(jobs []ai.HiringJob) ([]ai.ProspectFit, error) {
		calls++
		if len(jobs) > maxHiringScreened {
			t.Fatalf("batch of %d, limit %d", len(jobs), maxHiringScreened)
		}
		fits := make([]ai.ProspectFit, 0, len(jobs))
		for i := 1; i < len(jobs); i++ { // drop the first of every batch
			fits = append(fits, ai.ProspectFit{Index: i})
		}
		return fits, nil
	})
	if err != nil || calls != 2 || len(got) != 2 || got[many[0].ID] == "" || got[many[maxHiringScreened].ID] == "" {
		t.Fatalf("calls = %d, reasons = %v, err = %v; want the first lead of each of 2 batches dropped", calls, got, err)
	}

	// A valid but empty verdict on a batch of five or more archives nobody:
	// the batch is reported for review. A smaller batch is taken at its word.
	five := make([]database.CompanyLead, rescreenReviewBatch)
	for i := range five {
		five[i] = database.CompanyLead{ID: string(rune('a' + i)), Signal: "hiring", JobTitle: "Engineer"}
	}
	five = append(five, database.CompanyLead{ID: "ruled-out", Signal: "hiring", JobTitle: "Engineer", NotABuyer: "Sells a rival product."})
	none := func([]ai.HiringJob) ([]ai.ProspectFit, error) { return nil, nil }
	got, review, err := rescreenReasons(five, none)
	if err != nil || review != rescreenReviewBatch || len(got) != 1 || got["ruled-out"] == "" {
		t.Fatalf("empty verdict on %d: reasons = %v, review = %d, err = %v; want only the researched one archived", rescreenReviewBatch, got, review, err)
	}
	if got, review, err := rescreenReasons(five[:rescreenReviewBatch-1], none); err != nil || review != 0 || len(got) != rescreenReviewBatch-1 {
		t.Fatalf("a small batch: reasons = %v, review = %d, err = %v", got, review, err)
	}
}
