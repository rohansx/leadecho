package monitor

import "testing"

// A workspace with profiles but no embedded pain points (onboarded before an
// embedding key was set, or an embed call failed) must not drop every post
// as "low similarity": there is nothing to compare against.
func TestLowPainPointSimilarity(t *testing.T) {
	cases := []struct {
		name    string
		matches int
		best    float64
		want    bool
	}{
		{"no embedded pain points: classify everything", 0, 0, false},
		{"embedded, close match: keep", 3, 0.62, false},
		{"embedded, at threshold: keep", 3, 0.40, false},
		{"embedded, far: skip", 3, 0.21, true},
	}
	for _, c := range cases {
		if got := lowPainPointSimilarity(c.matches, c.best); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScoreStage1Rules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"too short", "short", false},
		{"spam pattern", "Click here to win a prize! " + string(make([]byte, 50)), false},
		{"valid post", "We are a 10-person team looking for a CRM that integrates with Reddit monitoring. Budget is around $100/mo.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scoreStage1Rules(tt.content); got != tt.want {
				t.Errorf("scoreStage1Rules() = %v, want %v", got, tt.want)
			}
		})
	}
}
