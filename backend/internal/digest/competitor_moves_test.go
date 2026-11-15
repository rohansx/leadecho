package digest

import (
	"fmt"
	"strings"
	"testing"
)

func TestTopCompetitorMovesPutsHighFirstAndCapsAtFive(t *testing.T) {
	var all []CompetitorMove
	for i := 0; i < 6; i++ {
		all = append(all, CompetitorMove{Competitor: "Acme", Title: fmt.Sprintf("normal %d", i)})
	}
	all = append(all, CompetitorMove{Competitor: "Beta", Title: "price rise", High: true})
	got := TopCompetitorMoves(all)
	if len(got) != MaxDigestMoves || MaxDigestMoves != 5 {
		t.Fatalf("got %d moves", len(got))
	}
	if got[0].Title != "price rise" || got[1].Title != "normal 0" {
		t.Fatalf("high first, then the given order: %+v", got)
	}
}

func TestCompetitorMovesInDigest(t *testing.T) {
	d := sampleData()
	d.CompetitorMoves = []CompetitorMove{{
		Competitor: "Acme <b>", Kind: "pricing", Title: "Starter raised to $49", Impact: "You are cheaper now.",
		URL: "https://acme.com/pricing", High: true,
	}}
	html, err := RenderHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Competitor moves", "Acme &lt;b&gt;", "Starter raised to $49", "You are cheaper now.", "https://acme.com/pricing", "/competitors"} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
	slack := RenderSlack(d)
	if !strings.Contains(slack, "Competitor moves") || !strings.Contains(slack, "Starter raised to $49") {
		t.Errorf("slack missing the moves:\n%s", slack)
	}
	if facts := Facts(d); !strings.Contains(facts, "competitor moves:") || !strings.Contains(facts, "Acme <b> (pricing): Starter raised to $49") {
		t.Errorf("facts missing the moves:\n%s", facts)
	}

	none := sampleData()
	html, _ = RenderHTML(none)
	if strings.Contains(html, "Competitor moves") || strings.Contains(RenderSlack(none), "Competitor moves") {
		t.Error("with no moves the section is left out")
	}
}

func TestCompetitorMoveAloneMakesADigestWorthSending(t *testing.T) {
	d := Data{Kind: KindWeekly}
	if !d.Empty() {
		t.Fatal("precondition: an empty digest")
	}
	d.CompetitorMoves = []CompetitorMove{{Competitor: "Acme", Title: "x"}}
	if d.Empty() {
		t.Fatal("a week with a competitor move is not empty")
	}
}
