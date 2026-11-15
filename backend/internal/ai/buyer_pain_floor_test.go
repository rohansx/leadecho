package ai

import "testing"

func TestBuyerPainFloor(t *testing.T) {
	cases := []struct {
		name  string
		role  string
		tags  []string
		pt    string
		score float64
		want  float64
	}{
		{"buyer pain at 5 is a lead", "buyer", []string{"pain_point"}, "discussion", 5, 6},
		{"buyer ask at 4.5 is a lead", "buyer", []string{"recommendation_request"}, "question", 4.5, 6},
		{"buyer opinion stays", "buyer", []string{"content_opportunity"}, "discussion", 5, 5},
		{"below 4.5 stays", "buyer", []string{"pain_point"}, "discussion", 4, 4},
		{"already a lead keeps its score", "buyer", []string{"pain_point"}, "discussion", 7, 7},
		{"unknown author stays", "unknown", []string{"pain_point"}, "discussion", 5, 5},
		{"non-buyer stays capped", "non_buyer", []string{"pain_point"}, "discussion", 5, 4},
		{"thought leadership stays capped", "buyer", []string{"pain_point"}, "thought_leadership", 5, 3},
	}
	for _, c := range cases {
		cr := NormalizeClassifyResult(ClassifyResult{AuthorRole: c.role, IntentTags: c.tags, PostType: c.pt, RelevanceScore: c.score}, "")
		if cr.RelevanceScore != c.want {
			t.Errorf("%s: score %.1f, want %.1f (intent %q)", c.name, cr.RelevanceScore, c.want, cr.Intent)
		}
	}
}
