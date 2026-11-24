package monitor

import (
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

func TestQualifiesAsLead(t *testing.T) {
	cases := []struct {
		score  float64
		intent string
		want   bool
	}{
		{6.0, "recommendation_ask", true},
		{6.5, "comparison", true},
		{9, "buy_signal", true},
		{6.5, "complaint", true},
		{5.9, "buy_signal", false},
		{9, "general", false},
		{8, "", false},
	}
	for _, c := range cases {
		if got := qualifiesAsLead(c.score, leadScoreThreshold, c.intent); got != c.want {
			t.Errorf("qualifiesAsLead(%v, %q) = %v, want %v", c.score, c.intent, got, c.want)
		}
	}
	// the workspace's strictness moves the line
	if qualifiesAsLead(7, 7.5, "buy_signal") || !qualifiesAsLead(5, 5, "buy_signal") {
		t.Error("qualifiesAsLead ignores the workspace's lead line")
	}
}

// A post with nothing open for its author (open_need "none") is not a lead at
// any strictness: its capped score must sit under the loosest lead line.
func TestNoOpenNeedIsUnderEveryLeadLine(t *testing.T) {
	r := ai.NormalizeClassifyResult(ai.ClassifyResult{IntentTags: []string{"pain_point"}, RelevanceScore: 8, AuthorRole: "buyer", OpenNeed: "none"}, "t")
	for _, s := range []database.LeadStrictness{database.LeadLoose, database.LeadBalanced, database.LeadStrict} {
		if qualifiesAsLead(r.RelevanceScore, s.MinScore(), r.Intent) {
			t.Errorf("%s: a post with nothing open qualified as a lead (score %v)", s, r.RelevanceScore)
		}
	}
	// An evaluation the model tagged only competitor_mention is a lead.
	r = ai.NormalizeClassifyResult(ai.ClassifyResult{IntentTags: []string{"competitor_mention"}, RelevanceScore: 6.5, AuthorRole: "buyer", OpenNeed: "evaluating"}, "t")
	if !qualifiesAsLead(r.RelevanceScore, database.LeadBalanced.MinScore(), r.Intent) {
		t.Errorf("an open evaluation at 6.5 did not qualify: intent %q", r.Intent)
	}
}
