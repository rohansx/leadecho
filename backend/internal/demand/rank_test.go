package demand

import (
	"strings"
	"testing"
	"time"
)

func who(id, author string, buyer, nonBuyer bool, at time.Time) Ask {
	return Ask{MentionID: id, Quote: "need " + id, Buyer: buyer, NonBuyer: nonBuyer, Platform: "reddit", Author: author, At: at}
}

func theme(id, status string, asks ...Ask) Theme {
	return Theme{ID: id, Title: "Theme " + id, Status: status, Coverage: CoverageGap, Asks: asks}
}

// Themes are ranked by how many BUYERS asked, then by how many people; a
// theme full of candidates does not outrank one two buyers raised.
func TestRankedPutsBuyerAsksFirst(t *testing.T) {
	b := func(id string) Ask { return who(id, "b_"+id, true, false, t0) }
	c := func(id string) Ask { return who(id, "c_"+id, false, true, t0) }
	u := func(id string) Ask { return who(id, "u_"+id, false, false, t0) }
	tests := []struct {
		name   string
		themes []Theme
		want   string
	}{
		{"buyers beat volume", []Theme{
			theme("candidates", StatusOpen, c("1"), c("2"), c("3"), c("4"), c("5"), c("6")),
			theme("buyers", StatusOpen, b("7"), b("8")),
		}, "buyers candidates"},
		{"same buyers: more people first", []Theme{
			theme("small", StatusOpen, b("1"), u("2")),
			theme("large", StatusOpen, b("3"), c("4"), u("5")),
		}, "large small"},
		{"no buyers anywhere: by people", []Theme{
			theme("two", StatusOpen, c("1"), u("2")),
			theme("three", StatusOpen, c("3"), c("4"), u("5")),
		}, "three two"},
		{"shipped last whatever its buyers", []Theme{
			theme("shipped", StatusShipped, b("1"), b("2"), b("3")),
			theme("open", StatusOpen, u("4"), u("5")),
		}, "open shipped"},
		{"ties keep their stored order", []Theme{
			theme("first", StatusOpen, b("1"), u("2")),
			theme("second", StatusOpen, b("3"), u("4")),
		}, "first second"},
	}
	for _, tt := range tests {
		var got []string
		for _, th := range (State{Themes: tt.themes}).Ranked() {
			got = append(got, th.ID)
		}
		if strings.Join(got, " ") != tt.want {
			t.Errorf("%s: order = %v, want %s", tt.name, got, tt.want)
		}
	}
}

// Each theme says how many of its people are buyers, how many are not, and
// (by difference) how many are unknown.
func TestViewCountsBuyersAndNonBuyers(t *testing.T) {
	s := State{Themes: []Theme{theme("t1", StatusOpen,
		who("1", "a", true, false, t0), who("2", "b", false, true, t0), who("3", "c", false, true, t0), who("4", "d", false, false, t0))}}
	v := s.View(t0)[0]
	if v.People != 4 || v.Buyers != 1 || v.NonBuyers != 2 {
		t.Fatalf("people=%d buyers=%d non_buyers=%d, want 4/1/2", v.People, v.Buyers, v.NonBuyers)
	}
}

// Themes one person raised are folded into a single "Other asks" group.
func TestFoldPutsNeedsRaisedOnceIntoOneGroup(t *testing.T) {
	old, mid, fresh := t0.Add(-20*24*time.Hour), t0.Add(-3*24*time.Hour), t0.Add(-time.Hour)
	s := State{Themes: []Theme{
		theme("once-buyer", StatusOpen, who("1", "a", true, false, mid)),
		theme("pair", StatusOpen, who("2", "b", false, true, old), who("3", "c", false, false, old)),
		theme("once-candidate", StatusOpen, who("4", "d", false, true, fresh)),
		theme("buyers", StatusOpen, who("5", "e", true, false, old), who("6", "f", true, false, old)),
		theme("once-shipped", StatusShipped, who("7", "g", true, false, old)),
		theme("once-old", StatusOpen, who("8", "h", false, false, old)),
	}}
	themes, other := Fold(s.View(t0))

	var ids []string
	for _, v := range themes {
		ids = append(ids, v.ID)
	}
	if got := strings.Join(ids, " "); got != "buyers pair once-shipped" {
		t.Fatalf("themes = %q, want the themes several people raised (most buyers first), then what shipped", got)
	}
	if other == nil {
		t.Fatal("the three needs raised once must be folded into one group")
	}
	if other.ID != OtherID || other.Title != "Other asks" || other.People != 3 || other.Buyers != 1 || other.NonBuyers != 1 || other.NewThisWeek != 2 {
		t.Fatalf("other = %+v", other)
	}
	var order []string
	for _, a := range other.Asks {
		order = append(order, a.MentionID)
	}
	if got := strings.Join(order, " "); got != "4 1 8" {
		t.Fatalf("other asks = %q, want newest first", got)
	}

	// Nothing raised once: no group at all.
	themes, other = Fold((State{Themes: []Theme{s.Themes[1], s.Themes[3]}}).View(t0))
	if other != nil || len(themes) != 2 {
		t.Fatalf("no singles: themes=%d other=%v", len(themes), other)
	}
	if themes, other = Fold(nil); other != nil || len(themes) != 0 {
		t.Fatalf("no themes: %v %v", themes, other)
	}
}

// One author, one role: the scorer judges each post alone, so the same
// person could be a buyer in one theme and not in the next.
func TestApplyGivesAnAuthorOneRole(t *testing.T) {
	state := State{Themes: []Theme{
		theme("t1", StatusOpen, who("m1", "Dana", true, false, t0), who("m2", "sam", false, true, t0), who("m3", "kim", true, false, t0)),
		theme("t2", StatusOpen, who("m4", "dana", false, false, t0), who("m5", "Sam", false, true, t0)),
	}}
	asks := []Ask{
		who("m6", "dana", false, true, t0), // Dana: buyer once, not a buyer once, unknown once
		who("m7", "sam", true, false, t0),  // Sam: not a buyer twice, buyer once
		who("m8", "kim", false, false, t0), // Kim: buyer once, unknown once
		who("m9", "", true, false, t0),     // no author: left as scored
		{MentionID: "m10", Quote: "x", Platform: "twitter", Author: "kim", NonBuyer: true, At: t0}, // another platform's "kim" is someone else
	}
	got := Apply(state, asks, []Assignment{{ThemeID: "t2", Asks: []int{0, 1, 2, 3, 4}}}, t0)

	role := map[string]string{}
	for _, th := range got.Themes {
		for _, a := range th.Asks {
			switch {
			case a.Buyer && a.NonBuyer:
				t.Fatalf("%s is both a buyer and not a buyer", a.MentionID)
			case a.Buyer:
				role[a.MentionID] = "buyer"
			case a.NonBuyer:
				role[a.MentionID] = "non_buyer"
			default:
				role[a.MentionID] = "unknown"
			}
		}
	}
	want := map[string]string{
		"m1": "unknown", "m4": "unknown", "m6": "unknown", // an even split is not a buyer: nobody is written to on a guess
		"m2": "non_buyer", "m5": "non_buyer", "m7": "non_buyer",
		"m3": "buyer", "m8": "buyer",
		"m9": "buyer", "m10": "non_buyer",
	}
	for id, w := range want {
		if role[id] != w {
			t.Errorf("%s: role = %s, want %s", id, role[id], w)
		}
	}
	if !state.Themes[0].Asks[0].Buyer || state.Themes[1].Asks[0].Buyer {
		t.Error("Apply must not change the state it was given")
	}
	// The counts and the follow-ups read the same flags.
	if b := got.Themes[0].Buyers(); b != 1 {
		t.Errorf("t1 buyers = %d, want 1 (kim)", b)
	}
	for _, a := range got.Themes[1].FollowUpTargets(MaxFollowUps, t0) {
		if a.Author == "sam" || strings.EqualFold(a.Author, "dana") {
			t.Errorf("%s (%s) must not get a follow-up", a.Author, a.MentionID)
		}
	}
}

// "Covered" means the product does what was asked. An ask that argues
// against the product's approach is an objection, and a theme judged under
// the older, looser rules is judged again.
func TestCoverageObjectionAndJudgingAgain(t *testing.T) {
	sys, _ := BuildPrompt(Product{Name: "P"}, nil, nil)
	for _, want := range []string{`"objection"`, "does the specific thing that was asked", "never \"covered\"", "the product's pitch, not their need", "[judge again]"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}

	old := theme("t1", StatusOpen, who("m1", "a", false, true, t0), who("m2", "b", false, true, t0))
	old.Coverage, old.Why = CoverageCovered, "Both are about live coding sessions."
	current := theme("t2", StatusOpen, who("m3", "c", true, false, t0), who("m4", "d", true, false, t0))
	current.Coverage, current.Rubric = CoverageCovered, CoverageRubric
	shipped := theme("t3", StatusShipped, who("m5", "e", true, false, t0))
	shipped.Coverage = CoverageGap
	themes := []Theme{old, current, shipped}

	_, user := BuildPrompt(Product{Name: "P"}, themes, nil)
	if !strings.Contains(user, "id t1 [judge again]") || strings.Contains(user, "id t2 [judge again]") || strings.Contains(user, "id t3 [judge again]") {
		t.Fatalf("only the open theme judged under older rules is marked:\n%s", user)
	}

	raw := `{"themes":[
	 {"id":"t1","why":"The product is a timed live exercise; these people say timed live exercises are unfair.","coverage":"objection","asks":[]},
	 {"id":"t2","why":"changed my mind","coverage":"gap","asks":[]},
	 {"id":"t3","why":"x","coverage":"objection","asks":[]},
	 {"id":"","title":"Not being recorded","summary":"s","why":"w","coverage":"objection","asks":[0]}
	]}`
	as, err := ParseAssignment(raw, themes, 1)
	if err != nil || len(as) != 2 {
		t.Fatalf("want the re-judged theme and the new one, got %d (%v): %+v", len(as), err, as)
	}
	got := Apply(State{Themes: themes}, []Ask{who("m9", "z", false, true, t0)}, as, t0)
	if th := got.Themes[0]; th.Coverage != CoverageObjection || !strings.Contains(th.Why, "unfair") || th.Rubric != CoverageRubric || len(th.Asks) != 2 {
		t.Errorf("t1 must be judged again and keep its asks: %+v", th)
	}
	if th := got.Themes[1]; th.Coverage != CoverageCovered {
		t.Errorf("a theme judged under the current rules keeps its coverage, got %q", th.Coverage)
	}
	if th := got.Themes[2]; th.Coverage != CoverageGap {
		t.Errorf("a shipped theme is never judged again, got %q", th.Coverage)
	}
	if th := got.Themes[3]; th.Coverage != CoverageObjection || th.Rubric != CoverageRubric {
		t.Errorf("new theme: %+v", th)
	}
	// Judged once, it is not asked about again.
	if _, user := BuildPrompt(Product{Name: "P"}, got.Themes, nil); strings.Contains(user, "[judge again]") {
		t.Errorf("nothing is left to judge again:\n%s", user)
	}

	// An old theme that gets new asks but no coverage keeps what it had.
	as, _ = ParseAssignment(`{"themes":[{"id":"t1","asks":[0]}]}`, themes, 1)
	kept := Apply(State{Themes: themes}, []Ask{who("m9", "z", false, true, t0)}, as, t0)
	if th := kept.Themes[0]; th.Coverage != CoverageCovered || th.Rubric != 0 || len(th.Asks) != 3 {
		t.Errorf("no coverage given: the old label stays until it is judged: %+v", th)
	}
}
