package researcher

import (
	"encoding/json"
	"math"
	"strings"
	"unicode"
)

// ICPSignals are the inputs to the ICP fit heuristic, all available after
// scoring + GitHub enrichment.
type ICPSignals struct {
	ProfileSimilarity     float64 // best pain-point cosine similarity (0 = no profiles configured)
	RelevanceScore        float64 // stage-3 LLM relevance, 0–10
	ConversionProbability float64 // stage-3 LLM conversion probability, 0–1
	HasCompany            bool
	HasWebsite            bool
	PublicRepos           int

	// enrich: firmographics from treg (0 / "" = not enriched).
	CompanySize   int
	Industry      string
	IndustryMatch bool // Industry appears in the workspace's target audience
}

// sizeFit: ponytail: fixed SMB/mid-market sweet spot (11–1000). Upgrade path:
// a target company size on the brand profile.
func sizeFit(n int) float64 {
	switch {
	case n >= 11 && n <= 1000:
		return 1
	case (n >= 2 && n <= 10) || (n > 1000 && n <= 10000):
		return 0.7
	}
	return 0.4
}

// enrichedFirmo blends treg firmographics into the base firmographic score.
func enrichedFirmo(s ICPSignals, firmo float64) float64 {
	if s.CompanySize <= 0 && s.Industry == "" {
		return firmo
	}
	ent := 0.0
	if s.CompanySize > 0 {
		ent += 0.4 * sizeFit(s.CompanySize)
	}
	switch {
	case s.IndustryMatch:
		ent += 0.6
	case s.Industry != "":
		ent += 0.12
	}
	return 0.5*firmo + 0.5*ent
}

var genericIndustryWords = map[string]bool{"services": true, "computer": true, "products": true, "general": true, "other": true, "industry": true}

// IndustryMatches reports whether a meaningful word of industry (>= 4 letters,
// not generic) occurs in the workspace's ICP text.
func IndustryMatches(industry, target string) bool {
	target = strings.ToLower(target)
	for _, w := range strings.FieldsFunc(strings.ToLower(industry), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len(w) >= 4 && !genericIndustryWords[w] && strings.Contains(target, w) {
			return true
		}
	}
	return false
}

// ICPFitScore returns a 0–1 fit score.
//
// ponytail: fixed-weight heuristic over signals we already have. Ceiling: it
// knows nothing about the customer's actual ICP (size, industry, role). Upgrade
// path: let workspaces describe their ICP and have the LLM grade the Person360.
func ICPFitScore(s ICPSignals) float32 {
	intent := 0.6*clamp01(s.RelevanceScore/10) + 0.4*clamp01(s.ConversionProbability)

	firmo := 0.0
	if s.HasCompany {
		firmo += 0.5
	}
	if s.HasWebsite {
		firmo += 0.2
	}
	if s.PublicRepos > 0 {
		firmo += 0.3
	}
	firmo = enrichedFirmo(s, firmo)

	// Similarity below the 0.40 stage-2 gate never reaches here; map 0.40–0.80 → 0–1.
	if s.ProfileSimilarity <= 0 {
		return float32(round2(0.75*intent + 0.25*firmo))
	}
	match := clamp01((s.ProfileSimilarity - 0.40) / 0.40)
	return float32(round2(0.45*match + 0.35*intent + 0.20*firmo))
}

// similarityFromMeta reads best_similarity written by the scorer.
func similarityFromMeta(meta []byte) float64 {
	var m struct {
		BestSimilarity float64 `json:"best_similarity"`
	}
	if len(meta) == 0 || json.Unmarshal(meta, &m) != nil {
		return 0
	}
	return m.BestSimilarity
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
