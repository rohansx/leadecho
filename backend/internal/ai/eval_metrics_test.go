package ai

import (
	"math"
	"testing"
)

// evalMetrics is a confusion matrix for "is this a lead?" plus what the
// calls cost. The live feedback eval (feedback_eval_live_test.go) fills one
// per workspace, per label source and overall.
type evalMetrics struct {
	N         int     `json:"n"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
	Errors    int     `json:"errors"` // no result (API error, refusal); not in the matrix
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	FPRate    float64 `json:"false_positive_rate"`
	CostUSD   float64 `json:"cost_usd"`
	Tokens    Usage   `json:"tokens"`
}

// add counts one case: want and got are "is a lead".
func (m *evalMetrics) add(want, got bool) {
	m.N++
	switch {
	case want && got:
		m.TP++
	case !want && got:
		m.FP++
	case want && !got:
		m.FN++
	default:
		m.TN++
	}
}

// finish fills the ratios. An empty denominator gives NaN-free 0 so the
// report stays valid JSON; read it next to the counts.
func (m *evalMetrics) finish() {
	m.Precision = ratio(m.TP, m.TP+m.FP)
	m.Recall = ratio(m.TP, m.TP+m.FN)
	m.FPRate = ratio(m.FP, m.FP+m.TN)
	m.CostUSD = math.Round(m.CostUSD*1e4) / 1e4
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return math.Round(float64(a)/float64(b)*1e4) / 1e4
}

// evalPrices are USD per 1M tokens {input, output}, copied from
// llm/cost.go (ai cannot import llm). Cache reads bill 0.1x, writes 1.25x.
var evalPrices = map[string][2]float64{
	"claude-opus-5-5":   {4, 20},
	"claude-sonnet-5-5": {2, 10},
	"claude-haiku-5-5":  {0.10, 0.50},
}

func evalCostUSD(model string, u Usage) float64 {
	p := evalPrices[model]
	in := float64(u.PromptTokens) + 0.1*float64(u.CacheReadTokens) + 1.25*float64(u.CacheWriteTokens)
	return (in*p[0] + float64(u.CompletionTokens)*p[1]) / 1e6
}

func TestEvalMetrics(t *testing.T) {
	var m evalMetrics
	// 3 TP, 1 FP, 4 TN, 2 FN
	for _, c := range []struct{ want, got bool }{
		{true, true}, {true, true}, {true, true},
		{false, true},
		{false, false}, {false, false}, {false, false}, {false, false},
		{true, false}, {true, false},
	} {
		m.add(c.want, c.got)
	}
	m.finish()
	if m.N != 10 || m.TP != 3 || m.FP != 1 || m.TN != 4 || m.FN != 2 {
		t.Fatalf("counts = %+v", m)
	}
	if m.Precision != 0.75 || m.Recall != 0.6 || m.FPRate != 0.2 {
		t.Fatalf("precision %v recall %v fpr %v, want 0.75 0.6 0.2", m.Precision, m.Recall, m.FPRate)
	}

	var empty evalMetrics
	empty.finish()
	if empty.Precision != 0 || empty.Recall != 0 || empty.FPRate != 0 {
		t.Fatalf("empty metrics should be 0, got %+v", empty)
	}

	// 1M uncached in + 1M cache read + 1M out on Opus: 4 + 0.4 + 20.
	got := evalCostUSD("claude-opus-5-5", Usage{PromptTokens: 1e6, CacheReadTokens: 1e6, CompletionTokens: 1e6})
	if math.Abs(got-24.4) > 1e-9 {
		t.Fatalf("cost = %v, want 24.4", got)
	}
}
