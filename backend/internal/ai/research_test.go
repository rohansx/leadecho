package ai

import (
	"strings"
	"testing"
)

func TestResearchPromptNamesTheCompanyAndTreatsTheWebAsUntrusted(t *testing.T) {
	system, user := buildResearchPrompt(CompanyResearch{
		Product: "Utkrusht", Overview: "Assesses engineers on real work.",
		Company: "Vista Higher Learning", CompanyURL: "https://vistahigherlearning.com", JobTitle: "Senior Backend Engineer",
		ContactName: "Dana Smith", ContactTitle: "VP Engineering",
	})
	if _, long := buildResearchPrompt(CompanyResearch{Company: strings.Repeat("x", 5000)}); len(long) > 1500 {
		t.Errorf("a scraped company name must be clipped, prompt is %d chars", len(long))
	}
	for _, want := range []string{"Vista Higher Learning", "vistahigherlearning.com", "Senior Backend Engineer", "Utkrusht", "VP Engineering"} {
		if !strings.Contains(user, want) {
			t.Errorf("user message is missing %q", want)
		}
	}
	for _, want := range []string{"untrusted", "only", "JSON"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
	// The contact is a private person: the research is about the company.
	if strings.Contains(user, "Dana Smith") {
		t.Error("the contact's name must not be sent to a web search")
	}
}

func TestParseResearchBriefKeepsOnlyFactsWithASearchedSource(t *testing.T) {
	seen := map[string]bool{"https://techcrunch.com/vista-raises": true, "https://vistahigherlearning.com/about": true}
	raw := "```json\n" + `{"what_they_do":"Publishes language-learning courseware for schools.",
"why_now":"Raised a growth round in March and is hiring four backend engineers.",
"hook":"Four backend roles at once means many interviews for a small team.",
"facts":[
 {"text":"Raised $40M in March 2026.","url":"https://techcrunch.com/vista-raises"},
 {"text":"Has 500 employees.","url":"https://made-up.example/size"},
 {"text":"Sells to K-12 and universities.","url":"https://vistahigherlearning.com/about/"},
 {"text":"No source at all.","url":""},
 {"text":"Script link.","url":"javascript:alert(1)"}
]}` + "\n```"
	b, err := parseResearchBrief(raw, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Facts) != 2 || b.Facts[0].URL != "https://techcrunch.com/vista-raises" || b.Facts[1].Text != "Sells to K-12 and universities." ||
		b.Facts[1].URL != "https://vistahigherlearning.com/about" { // the link the search returned, not the model's spelling of it
		t.Fatalf("facts = %+v", b.Facts)
	}
	if !strings.HasPrefix(b.WhatTheyDo, "Publishes") || !strings.HasPrefix(b.WhyNow, "Raised") || !strings.HasPrefix(b.Hook, "Four") {
		t.Fatalf("brief = %+v", b)
	}
}

func TestParseResearchBriefRejectsUnsourcedOrBrokenOutput(t *testing.T) {
	seen := map[string]bool{"https://a.example/x": true}
	for name, raw := range map[string]string{
		"not json":         "I could not find anything.",
		"no sourced facts": `{"what_they_do":"x","why_now":"y","hook":"z","facts":[{"text":"t","url":"https://other.example"}]}`,
		"nothing found":    `{"what_they_do":"","why_now":"","hook":"","facts":[]}`,
	} {
		if b, err := parseResearchBrief(raw, seen); err == nil {
			t.Errorf("%s: want an error, got %+v", name, b)
		}
	}
	// Over-long text is clipped, not rejected.
	long := `{"what_they_do":"` + strings.Repeat("a", 2000) + `","why_now":"y","hook":"z","facts":[{"text":"t","url":"https://a.example/x"}]}`
	b, err := parseResearchBrief(long, seen)
	if err != nil || len([]rune(b.WhatTheyDo)) > maxBriefField+1 {
		t.Fatalf("err=%v len=%d", err, len([]rune(b.WhatTheyDo)))
	}
}

func TestOutreachUsesTheResearchBrief(t *testing.T) {
	plain := outreachUserMessage(Outreach{Product: "Utkrusht", Company: "Vista", JobTitle: "Backend Engineer", ContactName: "Dana", ContactTitle: "VP Eng"})
	if strings.Contains(plain, "What we found") {
		t.Fatal("no research: no research section")
	}
	withBrief := outreachUserMessage(Outreach{
		Product: "Utkrusht", Company: "Vista", JobTitle: "Backend Engineer", ContactName: "Dana", ContactTitle: "VP Eng",
		Research: &ResearchBrief{WhatTheyDo: "Publishes courseware.", WhyNow: "Hiring four backend engineers.", Hook: "Many interviews for a small team.",
			Facts: []ResearchFact{{Text: "Raised $40M.", URL: "https://techcrunch.com/x"}}},
	})
	for _, want := range []string{"What we found", "Publishes courseware.", "Raised $40M."} {
		if !strings.Contains(withBrief, want) {
			t.Errorf("outreach input is missing %q", want)
		}
	}
	// Only what has a source may end up in an email: the model's own
	// summary lines (why now, the hook) are for the reader of the lead.
	for _, not := range []string{"Hiring four backend engineers.", "Many interviews"} {
		if strings.Contains(withBrief, not) {
			t.Errorf("unsourced line %q must not reach the draft", not)
		}
	}
	if !strings.Contains(outreachPrompt, "found about the company") {
		t.Error("the outreach prompt must say how to use the research")
	}
}
