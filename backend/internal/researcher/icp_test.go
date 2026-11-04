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

// enrich: company size / industry from treg enrichment feed the firmographic part.
func TestICPFitScoreFirmographics(t *testing.T) {
	base := ICPSignals{RelevanceScore: 9, ConversionProbability: 1, HasCompany: true} // 0.83 without enrichment
	cases := []struct {
		name string
		in   ICPSignals
		want float32
	}{
		{"no enrichment unchanged", base, 0.83},
		{"sweet-spot size + industry match", withFirmo(base, 120, true), 0.89},
		{"huge company, industry mismatch", withFirmo(base, 50000, false), 0.80},
		{"solo, industry match", withFirmo(base, 1, true), 0.86},
	}
	for _, c := range cases {
		if got := ICPFitScore(c.in); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func withFirmo(s ICPSignals, size int, match bool) ICPSignals {
	s.CompanySize, s.Industry, s.IndustryMatch = size, "Software", match
	return s
}

func TestIndustryMatches(t *testing.T) {
	cases := []struct {
		industry, target string
		want             bool
	}{
		{"Computer Software", "B2B SaaS founders and software teams", true},
		{"Financial Services", "fintech startups and financial controllers", true},
		{"Restaurants", "B2B SaaS founders", false},
		{"IT Services", "it services for everyone", false}, // generic / too short words never match
		{"", "anything", false},
	}
	for _, c := range cases {
		if got := IndustryMatches(c.industry, c.target); got != c.want {
			t.Errorf("IndustryMatches(%q,%q) = %v", c.industry, c.target, got)
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
