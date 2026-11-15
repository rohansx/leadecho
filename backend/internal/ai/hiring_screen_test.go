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

// A funded company has no posting: the screen is told so, with what there is
// to judge it by.
func TestHiringScreenMessageMarksFundedCompanies(t *testing.T) {
	msg := hiringScreenMessage(HiringScreen{Roles: []string{"Software Engineer"}, Jobs: []HiringJob{
		{Company: "Acme", Title: "Backend Engineer"},
		{Company: "Drillbit", Funding: "a Seed round of $4.4 million, announced 2026-11-13", Website: "drillbit.com", SourceURL: "https://news.example/drillbit-raises"},
	}})
	want := "1. [FUNDED] Drillbit raised a Seed round of $4.4 million, announced 2026-11-13. Website: drillbit.com. Announcement: https://news.example/drillbit-raises\n"
	if !strings.Contains(msg, want) || !strings.Contains(msg, "0. Acme: Backend Engineer\n") {
		t.Errorf("message:\n%s", msg)
	}
	if !strings.Contains(hiringScreenPrompt, "[FUNDED]") {
		t.Error("the screen's brief must say what a [FUNDED] entry is")
	}
}

// The round is the opening fact of a funding lead's message, and only as the
// stored fields give it; a posting's message says where the job is.
func TestOutreachForFundingAndJobLocation(t *testing.T) {
	round := "a Seed round of $4.4 million, announced 2026-11-13"
	funded := Outreach{Product: "P", ContactName: "Dana", ContactTitle: "CTO", Company: "Drillbit", Funding: round}
	if msg := outreachUserMessage(funded); !strings.Contains(msg, "They just raised: "+round) || strings.Contains(msg, "Job they posted") {
		t.Errorf("funding message:\n%s", msg)
	}
	if outreachSystemPrompt(funded) != fundingOutreachPrompt {
		t.Error("a round with no posting is written to about the round")
	}
	both := Outreach{Product: "P", ContactName: "Dana", Company: "Drillbit", JobTitle: "Software Engineer", JobLocation: "Rio de Janeiro, Brazil", Funding: round}
	msg := outreachUserMessage(both)
	if !strings.Contains(msg, "Job they posted this week: Software Engineer\nWhere the job is: Rio de Janeiro, Brazil") || !strings.Contains(msg, "They just raised: "+round) {
		t.Errorf("hiring message:\n%s", msg)
	}
	if outreachSystemPrompt(both) != outreachPrompt || outreachSystemPrompt(Outreach{SegmentPain: "x"}) != prospectOutreachPrompt {
		t.Error("a posting is the reason to write when there is one; nothing at all is a prospect")
	}
	// All three briefs carry the same promises, and a round's own figures pass
	// the number guard while a figure from research does not.
	for name, p := range map[string]string{"job": outreachPrompt, "funding": fundingOutreachPrompt, "prospect": prospectOutreachPrompt} {
		if !strings.Contains(p, outreachHonestyRules) {
			t.Errorf("the %s prompt lacks the shared honesty rules", name)
		}
	}
	if !strings.Contains(fundingOutreachPrompt, "You do not know that they are hiring") {
		t.Error("a funding message must not claim the company is hiring")
	}
	funded.Research = &ResearchBrief{Facts: []ResearchFact{{Text: "About 57 open roles.", URL: "https://example.com/jobs"}}}
	facts := funded
	facts.Research = nil
	if got := ungroundedNumbers("I saw you raised a $4.4 million Seed round on 13 November. You have 57 roles open.", outreachUserMessage(facts)); len(got) != 1 || got[0] != "57" {
		t.Errorf("ungrounded numbers in a funding draft = %v, want [57]", got)
	}
	if system, _ := outreachPrompts(Outreach{Funding: round, JobLocation: "Santiago, Chile"}); system != fundingOutreachPrompt {
		t.Error("without a job title there is no posting language to match")
	}
}
