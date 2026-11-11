package demand

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func ask(id, text string, buyer bool) Ask {
	return Ask{MentionID: id, Quote: text, Buyer: buyer, Platform: "reddit", URL: "https://example.com/" + id, Author: "u_" + id, At: t0}
}

func TestBuildPromptListsThemesAsksAndProduct(t *testing.T) {
	themes := []Theme{{ID: "t1", Title: "Screening for judgment", Summary: "Wants to test judgment, not syntax."}}
	asks := []Ask{ask("m1", "How do I screen seniors for judgment?", true), ask("m2", "Candidates use AI in live interviews", false)}
	sys, user := BuildPrompt(Product{Name: "Utkrusht", Overview: "Technical assessments.", Features: []string{"Tracks AI use"}}, themes, asks)

	for _, want := range []string{"JSON", "covered", "partial", "gap", "untrusted"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
	for _, want := range []string{"Utkrusht", "Tracks AI use", "t1", "Screening for judgment", "[0]", "How do I screen seniors for judgment?", "[1]", "(buyer)"} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt is missing %q", want)
		}
	}
}

func TestParseAssignment(t *testing.T) {
	themes := []Theme{{ID: "t1", Title: "Screening for judgment"}}
	raw := "```json\n" + `{"themes":[
	 {"id":"t1","asks":[0]},
	 {"id":"","title":"AI use in live interviews","summary":"Interviewers cannot tell who is using AI.","coverage":"covered","competitors":["HackerRank"],"asks":[1,1,7]},
	 {"id":"t9","title":"unknown id is treated as new","coverage":"nonsense","asks":[2]},
	 {"id":"","title":"","asks":[3]}
	]}` + "\n```"
	got, err := ParseAssignment(raw, themes, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 usable assignments (the untitled new theme is dropped), got %d: %+v", len(got), got)
	}
	if got[0].ThemeID != "t1" || len(got[0].Asks) != 1 || got[0].Asks[0] != 0 {
		t.Errorf("existing theme: %+v", got[0])
	}
	if got[1].ThemeID != "" || got[1].Title != "AI use in live interviews" || got[1].Coverage != CoverageCovered {
		t.Errorf("new theme: %+v", got[1])
	}
	if len(got[1].Asks) != 1 || got[1].Asks[0] != 1 {
		t.Errorf("duplicate and out-of-range indices must be dropped: %+v", got[1].Asks)
	}
	if got[2].ThemeID != "" || got[2].Coverage != CoverageGap {
		t.Errorf("an unknown id becomes a new theme and an unknown coverage becomes gap: %+v", got[2])
	}
	if _, err := ParseAssignment("not json", themes, 1); err == nil {
		t.Error("garbage must be an error")
	}
}

func TestApplyMergesAndCreatesThemes(t *testing.T) {
	state := State{Themes: []Theme{{ID: "t1", Title: "Screening for judgment", Status: StatusOpen, CreatedAt: t0.Add(-48 * time.Hour),
		Asks: []Ask{ask("m0", "old ask", true)}}}}
	asks := []Ask{ask("m1", "judgment again", true), ask("m2", "AI in interviews", false), ask("m0", "already there", true)}
	as := []Assignment{
		{ThemeID: "t1", Asks: []int{0, 2}},
		{Title: "AI use in live interviews", Summary: "Cannot tell who uses AI.", Coverage: CoverageCovered, Competitors: []string{"HackerRank"}, Asks: []int{1}},
	}
	got := Apply(state, asks, as, t0)

	if len(got.Themes) != 2 {
		t.Fatalf("want 2 themes, got %d", len(got.Themes))
	}
	t1 := got.Themes[0]
	if len(t1.Asks) != 2 || t1.Asks[1].MentionID != "m1" {
		t.Errorf("m1 should be added once and m0 not duplicated: %+v", t1.Asks)
	}
	if !t1.UpdatedAt.Equal(t0) {
		t.Errorf("updated_at = %v", t1.UpdatedAt)
	}
	nt := got.Themes[1]
	if nt.ID == "" || nt.ID == "t1" || nt.Status != StatusOpen || nt.Coverage != CoverageCovered || len(nt.Asks) != 1 {
		t.Errorf("new theme: %+v", nt)
	}
	if len(state.Themes) != 1 || len(state.Themes[0].Asks) != 1 {
		t.Error("Apply must not change the state it was given")
	}
	// A mention already in any theme is never added to a second one.
	again := Apply(got, []Ask{ask("m2", "AI in interviews", false)}, []Assignment{{ThemeID: "t1", Asks: []int{0}}}, t0)
	if len(again.Themes[0].Asks) != 2 {
		t.Errorf("m2 already belongs to another theme: %+v", again.Themes[0].Asks)
	}
}

func TestThemeCountsAndOrdering(t *testing.T) {
	th := Theme{Asks: []Ask{ask("a", "x", true), ask("b", "y", false), ask("c", "z", true)}}
	th.Asks[2].At = t0.Add(-10 * 24 * time.Hour)
	if th.People() != 3 || th.Buyers() != 2 {
		t.Fatalf("people=%d buyers=%d", th.People(), th.Buyers())
	}
	if got := th.NewSince(t0.Add(-7 * 24 * time.Hour)); got != 2 {
		t.Fatalf("new this week = %d, want 2", got)
	}
	state := State{Themes: []Theme{
		{ID: "small", Status: StatusOpen, Asks: []Ask{ask("a", "x", false)}},
		{ID: "shipped", Status: StatusShipped, Asks: []Ask{ask("b", "x", true), ask("c", "x", true), ask("d", "x", true)}},
		{ID: "big", Status: StatusOpen, Asks: []Ask{ask("e", "x", true), ask("f", "x", false)}},
	}}
	got := state.Ranked()
	if got[0].ID != "big" || got[1].ID != "small" || got[2].ID != "shipped" {
		t.Fatalf("open themes first, most buyers then most people; shipped last: %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestUnassignedSkipsKnownMentions(t *testing.T) {
	state := State{Themes: []Theme{{ID: "t1", Asks: []Ask{ask("m1", "x", true)}}}}
	got := state.Unassigned([]Ask{ask("m1", "x", true), ask("m2", "y", false), ask("m3", "  ", false)})
	if len(got) != 1 || got[0].MentionID != "m2" {
		t.Fatalf("want only m2 (m1 is assigned, m3 has no text), got %+v", got)
	}
}

func TestFollowUpTargetsAreBuyersNewestFirstAndCapped(t *testing.T) {
	th := Theme{}
	for i := 0; i < 30; i++ {
		a := ask(string(rune('A'+i)), "x", i%2 == 0)
		a.At = t0.Add(-time.Duration(i) * time.Hour)
		th.Asks = append(th.Asks, a)
	}
	got := th.FollowUpTargets(MaxFollowUps, t0)
	if len(got) != MaxFollowUps {
		t.Fatalf("want %d targets, got %d", MaxFollowUps, len(got))
	}
	for i, a := range got {
		if !a.Buyer {
			t.Fatalf("target %d is not a buyer", i)
		}
		if i > 0 && a.At.After(got[i-1].At) {
			t.Fatal("targets must be newest first")
		}
	}
	if n := len((Theme{Asks: []Ask{ask("m1", "x", false)}}).FollowUpTargets(MaxFollowUps, t0)); n != 0 {
		t.Fatalf("someone whose role is unknown gets no follow-up, got %d", n)
	}
}

func TestSeenAsksAreNotUnassignedAndTheListIsCapped(t *testing.T) {
	state := State{Seen: []string{"m2"}}
	got := state.Unassigned([]Ask{ask("m1", "x", true), ask("m2", "y", false)})
	if len(got) != 1 || got[0].MentionID != "m1" {
		t.Fatalf("m2 was already considered, got %+v", got)
	}
	// An ask pushed out of a full theme is remembered too, so it is not regrouped.
	full := Theme{ID: "t1", Status: StatusOpen}
	var more []Ask
	idx := []int{}
	for i := 0; i < MaxAsksPerTheme+2; i++ {
		more = append(more, ask("n"+string(rune('0'+i%10))+string(rune('a'+i/10)), "x", true))
		idx = append(idx, i)
	}
	next := Apply(State{Themes: []Theme{full}}, more, []Assignment{{ThemeID: "t1", Asks: idx}}, t0)
	if next.Themes[0].People() != MaxAsksPerTheme || len(next.Unassigned(more)) != 0 {
		t.Fatalf("people=%d unassigned=%d", next.Themes[0].People(), len(next.Unassigned(more)))
	}
	var many []Ask
	for i := 0; i < MaxSeen+50; i++ {
		many = append(many, ask("s"+string(rune(i)), "x", false))
	}
	if n := len(next.WithSeen(many).Seen); n != MaxSeen {
		t.Fatalf("seen list must be capped at %d, got %d", MaxSeen, n)
	}
}

func TestOutsideCoverageIsKept(t *testing.T) {
	as, err := ParseAssignment(`{"themes":[{"id":"","title":"A team ATS","summary":"s","why":"The product tests candidates; it does not track applicants.","coverage":"outside","asks":[0]}]}`, nil, 1)
	if err != nil || len(as) != 1 || as[0].Coverage != CoverageOutside {
		t.Fatalf("got %+v, %v", as, err)
	}
	// The reason is kept and shown, so the customer can check the label.
	next := Apply(State{}, []Ask{ask("m1", "an ATS", true)}, as, t0)
	if got := next.View(t0)[0].Why; got != "The product tests candidates; it does not track applicants." {
		t.Fatalf("why = %q", got)
	}
}

// A reply to a post from months ago is unwelcome: old asks get no follow-up.
func TestFollowUpTargetsSkipOldPosts(t *testing.T) {
	old := ask("m1", "x", true)
	old.At = t0.Add(-MaxFollowUpAge - time.Hour)
	fresh := ask("m2", "y", true)
	fresh.At = t0.Add(-24 * time.Hour)
	got := Theme{Asks: []Ask{old, fresh}}.FollowUpTargets(MaxFollowUps, t0)
	if len(got) != 1 || got[0].MentionID != "m2" {
		t.Fatalf("want only the recent ask, got %+v", got)
	}
}
