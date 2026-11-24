package keywordjudge

import (
	"slices"
	"strings"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/database"
)

func TestKeywordStatsIncludesSilentKeywords(t *testing.T) {
	kws := []database.ListKeywordsRow{
		{ID: "a", Term: "Wispr Flow alternative", IsActive: true, Platforms: []string{"twitter", "reddit"}},
		{ID: "b", Term: "Hinglish dictation", IsActive: true, Platforms: []string{"reddit"}},
	}
	rows := []database.KeywordYield{
		{KeywordID: "a", Platform: "twitter", Posts: 106, Judged: 46, Leads: 0},
		{KeywordID: "a", Platform: "reddit", Posts: 30, Judged: 12, Leads: 3},
	}
	got := keywordStats(kws, rows)
	if len(got) != 2 {
		t.Fatalf("want both keywords, got %d", len(got))
	}
	if got[0].Posts != 136 || got[0].Leads != 3 || len(got[0].ByPlatform) != 2 || !strings.HasPrefix(got[0].ByPlatform[0], "reddit") {
		t.Errorf("totals wrong: %+v", got[0])
	}
	if got[1].Posts != 0 || got[1].Term != "Hinglish dictation" {
		t.Errorf("a keyword with no posts must still be reviewed: %+v", got[1])
	}
}

// The loop: the next review is told what the customer did with each
// suggestion and how the added keywords have done since.
func TestPreviousTextClosesTheLoop(t *testing.T) {
	prev := &Stored{
		GeneratedAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
		Summary:     "Reddit works, X does not.",
		Suggestions: []Suggestion{
			{ID: suggestionID("add", "Hinglish dictation"), Kind: "add", Term: "Hinglish dictation"},
			{ID: suggestionID("pause", "Granola alternative"), Kind: "pause", Term: "Granola alternative"},
			{ID: suggestionID("keep", "Lungoor"), Kind: "keep", Term: "Lungoor"},
		},
		Decisions: map[string]string{suggestionID("add", "Hinglish dictation"): "applied"},
	}
	stats := []ai.KeywordStat{{Term: "hinglish dictation", Posts: 12, Leads: 2}}
	txt := previousText(prev, stats)
	for _, want := range []string{"2026-11-10", "Reddit works", `add "Hinglish dictation": applied; since then 12 posts, 2 leads`, `pause "Granola alternative": no decision yet`} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "Lungoor") {
		t.Errorf("keep verdicts are not advice to follow up on:\n%s", txt)
	}
	if previousText(nil, stats) != "" {
		t.Error("no previous review: no text")
	}
}

func TestToStoredGivesStableIDs(t *testing.T) {
	rev := &ai.KeywordReview{
		Summary:  "s",
		Add:      []ai.SuggestedKeyword{{Term: "hinglish dictation", Platforms: []string{"reddit"}}},
		Verdicts: []ai.KeywordVerdict{{Term: "Granola alternative", Action: "pause"}},
	}
	st := toStored(rev, []ai.KeywordStat{{Posts: 10, Leads: 1}, {Posts: 5}}, "weekly", time.Now())
	if st.Posts != 15 || st.Leads != 1 || len(st.Suggestions) != 2 {
		t.Fatalf("got %+v", st)
	}
	if st.Suggestions[0].ID != suggestionID("add", "Hinglish Dictation ") {
		t.Error("an id must not depend on case or spacing, so a decision survives a re-run")
	}
	if st.Decisions == nil || st.Scoring == nil {
		t.Error("empty maps and lists must encode as {} and [], not null")
	}
}

func TestPlatformsWithoutPaidData(t *testing.T) {
	for _, p := range Platforms(billing.LockedFor("free", false)) {
		if p == "twitter" || p == "linkedin" || p == "reddit" {
			t.Errorf("paid platform %q offered to a workspace without paid data", p)
		}
	}
	if len(Platforms(nil)) != len(ai.KeywordPlatforms) {
		t.Error("with nothing locked every platform is offered")
	}
	solo := Platforms(billing.LockedFor("solo", true))
	if !slices.Contains(solo, "reddit") || slices.Contains(solo, "twitter") || slices.Contains(solo, "linkedin") {
		t.Errorf("solo platforms = %v, want reddit without X and LinkedIn", solo)
	}
}
