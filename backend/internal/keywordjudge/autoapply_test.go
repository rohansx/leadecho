package keywordjudge

import (
	"testing"
	"time"

	"leadecho/internal/database"
)

func TestPlanAppliesOnlyWhatTheRulesAllow(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	old, young := now.Add(-8*24*time.Hour), now.Add(-2*24*time.Hour)
	kw := func(id, term string, created time.Time, ps ...string) database.ListKeywordsRow {
		return database.ListKeywordsRow{ID: id, Term: term, IsActive: true, CreatedAt: created, Platforms: ps}
	}
	kws := []database.ListKeywordsRow{
		kw("1", "dead keyword", old, "reddit"),
		kw("2", "new keyword", young, "reddit"),
		kw("3", "had a lead", old, "reddit"),
		kw("4", "too broad", old, "reddit", "devto"),
		kw("5", "a", old, "reddit"), kw("6", "b", old, "reddit"), kw("7", "c", old, "reddit"),
	}
	st := Stored{Decisions: map[string]string{"done": "dismissed"}, Suggestions: []Suggestion{
		{ID: "p1", Kind: "pause", Term: "Dead Keyword"},
		{ID: "p2", Kind: "pause", Term: "new keyword"},                                  // under a week old
		{ID: "p3", Kind: "pause", Term: "had a lead"},                                   // found a lead
		{ID: "m1", Kind: "platforms", Term: "too broad", Platforms: []string{"reddit"}}, // narrowed
		{ID: "m2", Kind: "platforms", Term: "a", Platforms: []string{"linkedin"}},       // not allowed
		{ID: "a1", Kind: "add", Term: "first 10 customers", Platforms: []string{"reddit"}},
		{ID: "a2", Kind: "add", Term: "c", Platforms: []string{"reddit"}},            // exists
		{ID: "a3", Kind: "add", Term: "no room left", Platforms: []string{"reddit"}}, // over the limit
		{ID: "done", Kind: "pause", Term: "b"},                                       // customer decided
		{ID: "k", Kind: "keep", Term: "a"},
		{ID: "m3", Kind: "platforms", Term: "had a lead", Platforms: []string{"devto"}}, // would drop the lead's platform
	}}
	got := map[string]action{}
	for _, a := range plan(st, kws, map[string]map[string]int{"3": {"reddit": 1}, "4": {"reddit": 2}}, []string{"reddit", "devto"}, 1, now) {
		got[a.suggestionID] = a
	}
	want := []string{"p1", "m1", "a1"}
	if len(got) != len(want) {
		t.Fatalf("applied %v, want %v", got, want)
	}
	for _, id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("%s not applied", id)
		}
	}
	if ps := got["m1"].platforms; len(ps) != 1 || ps[0] != "reddit" {
		t.Errorf("narrowed to %v", ps)
	}
}

func TestPlanKeepsMinimumActiveKeywords(t *testing.T) {
	now := time.Now()
	var kws []database.ListKeywordsRow
	var st Stored
	for _, id := range []string{"1", "2", "3", "4", "5", "6"} {
		kws = append(kws, database.ListKeywordsRow{ID: id, Term: "k" + id, IsActive: true, CreatedAt: now.Add(-30 * 24 * time.Hour)})
		st.Suggestions = append(st.Suggestions, Suggestion{ID: "p" + id, Kind: "pause", Term: "k" + id})
	}
	if n := len(plan(st, kws, nil, nil, 0, now)); n != 6-MinActive {
		t.Errorf("paused %d, want %d (keep %d active)", n, 6-MinActive, MinActive)
	}
}
