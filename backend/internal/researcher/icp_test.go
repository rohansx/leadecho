package researcher

import "testing"

func TestICPFitScore(t *testing.T) {
	cases := []struct {
		name string
		in   ICPSignals
		want float32
	}{
		{"empty", ICPSignals{}, 0},
		{"perfect", ICPSignals{ProfileSimilarity: 0.9, RelevanceScore: 10, ConversionProbability: 1, HasCompany: true, HasWebsite: true, PublicRepos: 3}, 1},
		{"strong match no firmographics", ICPSignals{ProfileSimilarity: 0.8, RelevanceScore: 8, ConversionProbability: 0.5}, 0.69},
		{"no profiles uses intent+firmo", ICPSignals{RelevanceScore: 9, ConversionProbability: 1, HasCompany: true}, 0.83},
		{"out of range inputs clamp", ICPSignals{ProfileSimilarity: 2, RelevanceScore: 50, ConversionProbability: -1}, 0.66},
	}
	for _, c := range cases {
		if got := ICPFitScore(c.in); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestSimilarityFromMeta(t *testing.T) {
	if got := similarityFromMeta([]byte(`{"best_similarity":0.61}`)); got != 0.61 {
		t.Errorf("got %v", got)
	}
	if got := similarityFromMeta([]byte(`not json`)); got != 0 {
		t.Errorf("bad json: got %v", got)
	}
}
