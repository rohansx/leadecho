package monitor

import (
	"strings"
	"testing"
)

// An open software-engineer role alone is not a reason to buy an
// engineering assessment tool; volume, a talent hire, a round or a startup
// posting on HN is.
func TestWithTriggers(t *testing.T) {
	jobs := []jobPost{
		{Company: "Unity", Title: "Senior Software Engineer", Openings: 1, Source: "linkedin"},
		{Company: "Acme", Title: "Backend Developer", Openings: 3, Source: "linkedin"},
		{Company: "Beta", Title: "Technical Recruiter", Openings: 1, Source: "linkedin"},
		{Company: "Gamma", Title: "Software Engineer", Openings: 1, Source: sourceHN},
		{Company: "Delta", Funding: &fundingRound{}, Source: "funding"},
	}
	kept, dropped := withTriggers(jobs)
	if dropped != 1 || len(kept) != 4 {
		t.Fatalf("kept %d, dropped %d", len(kept), dropped)
	}
	for _, j := range kept {
		if j.Company == "Unity" {
			t.Error("a single plain posting must not pass")
		}
		if j.Trigger == "" {
			t.Errorf("%s kept without a trigger", j.Company)
		}
	}
	if !strings.Contains(kept[0].Trigger, "3 matching openings") || !strings.Contains(kept[1].Trigger, "setting up its hiring process") {
		t.Errorf("triggers: %q / %q", kept[0].Trigger, kept[1].Trigger)
	}
	if jobs[1].Trigger != "" {
		t.Error("the input slice must not be modified")
	}
}
