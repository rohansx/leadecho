package ai

import (
	"strings"
	"testing"
)

func TestHiringScreenMessageNumbersJobsAndStatesTheMarket(t *testing.T) {
	msg := hiringScreenMessage(HiringScreen{
		Product: "Utkrusht", Overview: "Technical skill assessments.", Audience: "tech companies under 5,000 employees",
		Roles: []string{"Software Engineer"}, MaxCompanySize: 5000,
		Jobs: []HiringJob{
			{Company: "Acme", Title: "Backend Engineer", Location: "Austin, TX"},
			{Company: "Softys", Title: "ANALISTA DE AUTOMATIZACION"},
		},
	})
	for _, want := range []string{"Utkrusht", "under 5,000 employees", "5000 employees", "Software Engineer",
		"0. Acme: Backend Engineer (Austin, TX)", "1. Softys: ANALISTA DE AUTOMATIZACION"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(hiringScreenMessage(HiringScreen{Jobs: []HiringJob{{Company: "A", Title: "B"}}}), "employees") {
		t.Error("no size ceiling: none must be stated")
	}
}

// The research is the only check that reads what the company says about
// itself: it can find a staffing firm the screen let through.
func TestResearchBriefCarriesNotABuyer(t *testing.T) {
	searched := map[string]bool{"https://revature.com/about": true}
	raw := `{"what_they_do":"Trains and places developers at client companies.","why_now":"","hook":"","not_a_buyer":"It is a staffing firm that hires developers for its clients.","facts":[{"text":"Revature places trained developers with enterprise clients.","url":"https://revature.com/about"}]}`
	b, err := parseResearchBrief(raw, searched)
	if err != nil || !strings.Contains(b.NotABuyer, "staffing firm") {
		t.Fatalf("brief = %+v, err = %v", b, err)
	}
	b, err = parseResearchBrief(strings.Replace(raw, `"not_a_buyer":"It is a staffing firm that hires developers for its clients.",`, "", 1), searched)
	if err != nil || b.NotABuyer != "" {
		t.Fatalf("a fitting company has no verdict: %+v, %v", b, err)
	}
	_, user := buildResearchPrompt(CompanyResearch{Company: "Revature", Product: "Utkrusht", Audience: "tech companies under 5,000 employees", MaxCompanySize: 5000})
	if !strings.Contains(user, "Who buys it: tech companies under 5,000 employees") || !strings.Contains(user, "5000") {
		t.Fatalf("the research must know who buys the product:\n%s", user)
	}
}

// A buyer who describes the problem in their own team, without asking for a
// tool, is a lead (6+): in production these were scored 3 to 5.5 and lost.
func TestRubricScoresABuyerDescribingTheirOwnProblemAsALead(t *testing.T) {
	rules := productFitRules("P")
	for _, want := range []string{"6-7.5", "their own team or company", "workaround they built"} {
		if !strings.Contains(rules, want) {
			t.Errorf("rubric lacks %q", want)
		}
	}
	for _, want := range []string{"bears on this hire", "Never name the site a fact came from", "who looks after it"} {
		if !strings.Contains(outreachPrompt, want) {
			t.Errorf("outreach prompt lacks %q", want)
		}
	}
}
