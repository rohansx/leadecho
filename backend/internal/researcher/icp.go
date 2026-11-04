package researcher

import (
	"encoding/json"
	"math"
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
