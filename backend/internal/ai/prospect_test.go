package ai

import (
	"strings"
	"testing"
)

func TestParseProspectFits(t *testing.T) {
	raw := "```json\n{\"fits\":[{\"i\":3,\"why\":\"b\"},{\"i\":1,\"why\":\"a\"},{\"i\":1,\"why\":\"dup\"},{\"i\":9,\"why\":\"made up\"},{\"i\":-1}]}\n```"
	fits, err := parseProspectFits(raw, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(fits) != 2 || fits[0].Index != 1 || fits[0].Reason != "a" || fits[1].Index != 3 {
		t.Fatalf("fits = %+v", fits)
	}
	if _, err := parseProspectFits("no json", 5); err == nil {
		t.Fatal("want an error for a reply without JSON")
	}
}

func TestProspectFitMessage(t *testing.T) {
	msg := prospectFitMessage(ProspectCheck{
		Product: "PaperHQ", Overview: "year-end packs",
		Segment:    CustomerSegment{Name: "Franchise networks", Criteria: []string{"Multiple local entities"}, NegativeCriteria: []string{"Single-location business"}},
		Candidates: []ProspectCandidate{{Name: "Racz", Domain: "raczgroup.co.uk", Employees: 60, Description: "Franchise developer"}},
	})
	for _, want := range []string{"Multiple local entities", "Exclusions", "Single-location business", "0. Racz (raczgroup.co.uk), 60 employees: Franchise developer"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}

func TestOutreachMessageForProspect(t *testing.T) {
	msg := outreachUserMessage(Outreach{Product: "P", ContactName: "Abey", Company: "JRK", SegmentPain: "multi-site packs get messy"})
	if strings.Contains(msg, "Job they posted") || !strings.Contains(msg, "multi-site packs get messy") {
		t.Fatalf("prospect message:\n%s", msg)
	}
	msg = outreachUserMessage(Outreach{Product: "P", ContactName: "A", Company: "C", JobTitle: "Engineer", SegmentPain: "x"})
	if !strings.Contains(msg, "Job they posted this week: Engineer") || strings.Contains(msg, "often have") {
		t.Fatalf("hiring message:\n%s", msg)
	}
}
