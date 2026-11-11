package digest

import (
	"strings"
	"testing"
)

func TestDigestShowsWhatTheMarketAskedFor(t *testing.T) {
	d := Data{Kind: KindWeekly, Demand: []DemandTheme{
		{Title: "Telling who uses AI in a live interview", People: 9, Buyers: 4, NewThisWeek: 3, Coverage: "covered"},
		{Title: "Filtering a high volume of résumés", People: 5, Buyers: 2, NewThisWeek: 5, Coverage: "gap"},
	}}
	html, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"What your market asked for", "Telling who uses AI in a live interview", "9 people", "4 buyers", "you already do this", "Filtering a high volume of résumés", "not covered yet"} {
		if !strings.Contains(html, want) {
			t.Errorf("email is missing %q", want)
		}
	}
	slack := RenderSlack(d)
	if !strings.Contains(slack, "Asked for") || !strings.Contains(slack, "Filtering a high volume of résumés (5 people, not covered yet)") {
		t.Fatalf("slack = %q", slack)
	}
	// Nothing to show: no section at all.
	plain, _ := RenderHTML(Data{Kind: KindWeekly})
	if strings.Contains(plain, "What your market asked for") {
		t.Fatal("the section must not appear without themes")
	}
	if coverageLabel("partial") != "partly covered" || coverageLabel("anything") != "not covered yet" {
		t.Fatal("coverage labels")
	}
}
