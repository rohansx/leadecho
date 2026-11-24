package monitor

import "testing"

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
