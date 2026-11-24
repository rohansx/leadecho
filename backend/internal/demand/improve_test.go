package demand

import (
	"context"
	"strings"
	"testing"
	"time"
)

func improveState() State {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	return State{Themes: []Theme{
		{ID: "t1", Title: "Ban-safe Reddit replies", Coverage: CoverageGap, Status: StatusOpen, Asks: []Ask{
			{MentionID: "m-a", Quote: "how do I not get banned", Buyer: true, At: at},
			{MentionID: "m-b", Quote: "subreddit rules are a minefield", At: at},
		}},
		{ID: "t2", Title: "Cheaper plan", Coverage: CoveragePartial, Status: StatusOpen, Asks: []Ask{
			{MentionID: "m-c", Quote: "too pricey for me", At: at},
			{MentionID: "m-d", Quote: "need something under $30", At: at},
		}},
		{ID: "t3", Title: "Only one person", Coverage: CoverageGap, Status: StatusOpen, Asks: []Ask{{MentionID: "m-e", Quote: "x", At: at}}},
		{ID: "t4", Title: "Different product", Coverage: CoverageOutside, Status: StatusOpen, Asks: []Ask{{MentionID: "m-f", Quote: "x", At: at}, {MentionID: "m-g", Quote: "y", At: at}}},
	}}
}

func TestImproveInputSkipsSingleAndOutside(t *testing.T) {
	got := improveInput(improveState())
	if len(got) != 2 || got[0].ID != "t1" || got[1].ID != "t2" {
		t.Fatalf("themes = %+v, want t1, t2", got)
	}
}

func TestParseImprovementsValidates(t *testing.T) {
	themes := improveInput(improveState())
	_, input := BuildImprovePrompt(Product{Name: "P"}, themes, map[string]Signal{"m-d": {Budget: "need something under $30 a month"}})
	raw := "```json\n" + `{"improvements":[
		{"title":"Show subreddit rules","change":"Show each subreddit's self-promotion rules.","why":"2 people, 1 buyer.","for_whom":"founders","kind":"feature","themes":["t1","t9"],"evidence":["m-a","m-a","nope"]},
		{"title":"Uncited","change":"Do X.","why":"","kind":"feature","evidence":[]},
		{"title":"Unknown ids only","change":"Do Y.","kind":"feature","evidence":["m-zzz"]},
		{"title":"Made-up figure","change":"Charge $19.","why":"","kind":"pricing","evidence":["m-d"]},
		{"title":"Entry plan","change":"A plan under $30.","why":"","kind":"weird","evidence":["m-d"]}
	]}` + "\n```"
	got, err := ParseImprovements(raw, themes, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(got), got)
	}
	first := got[0]
	if len(first.Evidence) != 1 || first.Evidence[0].MentionID != "m-a" || first.Evidence[0].Quote != "how do I not get banned" || !first.Evidence[0].Buyer {
		t.Errorf("evidence = %+v, want m-a once with the stored quote", first.Evidence)
	}
	if strings.Join(first.Themes, ",") != "t1" {
		t.Errorf("themes = %v, want t1 (t9 is unknown)", first.Themes)
	}
	if got[1].Title != "Entry plan" || got[1].Kind != KindFeature || strings.Join(got[1].Themes, ",") != "t2" {
		t.Errorf("second = %+v, want Entry plan, kind defaulted, theme taken from its evidence", got[1])
	}
	if _, err := ParseImprovements("not json", themes, input); err == nil {
		t.Error("want an error for a reply that is not JSON")
	}
}

func TestImproveOnlyWhenInputChanged(t *testing.T) {
	reply := `{"improvements":[{"title":"Show subreddit rules","change":"Show the rules.","kind":"feature","evidence":["m-a"]}]}`
	store := &fakeStore{state: improveState()}
	model := &fakeModel{replies: []string{reply}}
	svc := &Service{Store: store, Model: model}
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	if ran, err := svc.Improve(context.Background(), "ws", now); err != nil || !ran {
		t.Fatalf("first run: ran=%v err=%v", ran, err)
	}
	imp := store.state.Improvements
	if imp == nil || len(imp.Items) != 1 || imp.InputHash == "" || !imp.GeneratedAt.Equal(now) {
		t.Fatalf("stored = %+v", imp)
	}
	if ran, _ := svc.Improve(context.Background(), "ws", now); ran || len(model.seen) != 1 {
		t.Fatalf("same input: ran=%v calls=%d, want no second call", ran, len(model.seen))
	}

	// A grouping run keeps the suggestions; a new ask changes the input.
	s := Apply(store.state, []Ask{{MentionID: "m-new", Quote: "rules please"}}, []Assignment{{ThemeID: "t1", Asks: []int{0}}}, now)
	if s.Improvements == nil {
		t.Fatal("Apply dropped the improvements")
	}
	store.state = s
	if ran, _ := svc.Improve(context.Background(), "ws", now); !ran || len(model.seen) != 2 {
		t.Fatalf("changed input: ran=%v calls=%d, want a second call", ran, len(model.seen))
	}

	// Nothing several people raised: no call at all.
	empty := &fakeModel{replies: []string{reply}}
	if ran, err := (&Service{Store: &fakeStore{}, Model: empty}).Improve(context.Background(), "ws", now); ran || err != nil || len(empty.seen) != 0 {
		t.Fatalf("no themes: ran=%v err=%v calls=%d", ran, err, len(empty.seen))
	}
}

func TestBudgetLine(t *testing.T) {
	for in, want := range map[string]string{
		"I'm pre-revenue and can spend maybe $20/mo on tools right now": "I'm pre-revenue and can spend maybe $20/mo on tools right now",
		"no money talk here":    "",
		"our budget is tight":   "our budget is tight",
		"we pay 50 dollars now": "we pay 50 dollars now",
	} {
		if got := budgetLine(in); got != want {
			t.Errorf("budgetLine(%q) = %q, want %q", in, got, want)
		}
	}
}
