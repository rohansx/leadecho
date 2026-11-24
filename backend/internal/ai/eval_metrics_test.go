package ai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// evalBudget checks the run can be costed and capped before any call is made:
// a model missing from evalPrices would cost $0 and never trip the cap, and
// an unparsable cap would silently become 0.
func evalBudget(model, rawMax string) (float64, error) {
	if _, ok := evalPrices[model]; !ok {
		return 0, fmt.Errorf("no price for model %q in evalPrices: add it so EVAL_MAX_USD can cap the run", model)
	}
	maxUSD, err := strconv.ParseFloat(strings.TrimSpace(rawMax), 64)
	if err != nil || math.IsNaN(maxUSD) || math.IsInf(maxUSD, 0) || maxUSD <= 0 {
		return 0, fmt.Errorf("EVAL_MAX_USD = %q: want a positive number of dollars", rawMax)
	}
	return maxUSD, nil
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

// evalCase is one line of cases.jsonl.
type evalCase struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"` // anonymised, e.g. ws_03
	Context   string `json:"context"`   // key into contexts.json
	Platform  string `json:"platform"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Label     string `json:"label"`  // lead | not_lead
	Source    string `json:"source"` // where the label came from
	Reason    string `json:"reason,omitempty"`
}

// withoutCase drops a few-shot example that is the case itself, so a post
// the workspace already labelled is not scored with its own answer. It keeps
// only the examples production would send (the first MaxScoringExamples), so
// a dropped example leaves its slot empty instead of letting a 7th in.
// Posts match the way formatExamples renders them (whitespace collapsed).
func withoutCase(sc ScoringContext, c evalCase) ScoringContext {
	same := func(e ScoringExample) bool {
		return truncate(e.Title, 150) == truncate(c.Title, 150) && truncate(e.Content, 400) == truncate(c.Content, 400)
	}
	in := sc.Examples
	if len(in) > MaxScoringExamples {
		in = in[:MaxScoringExamples]
	}
	var ex []ScoringExample
	for _, e := range in {
		if !same(e) {
			ex = append(ex, e)
		}
	}
	sc.Examples = ex
	return sc
}

func loadEvalSet(t *testing.T, dir string) ([]evalCase, map[string]ScoringContext) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "contexts.json"))
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[string]ScoringContext{}
	if err := json.Unmarshal(raw, &contexts); err != nil {
		t.Fatalf("contexts.json: %v", err)
	}
	f, err := os.Open(filepath.Join(dir, "cases.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []evalCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var c evalCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("cases.jsonl line %d: %v", len(cases)+1, err)
		}
		if c.Label != "lead" && c.Label != "not_lead" {
			t.Fatalf("%s: label %q", c.ID, c.Label)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases, contexts
}

func TestEvalBudget(t *testing.T) {
	if got, err := evalBudget(classifyEvalModel, "8"); err != nil || got != 8 {
		t.Fatalf("default = %v, %v", got, err)
	}
	if got, err := evalBudget("claude-haiku-5-5", " 2.5 "); err != nil || got != 2.5 {
		t.Fatalf("haiku 2.5 = %v, %v", got, err)
	}
	for _, c := range []struct{ model, max string }{
		{"claude-unknown-9", "8"}, // would cost $0 and never trip the cap
		{classifyEvalModel, "8usd"},
		{classifyEvalModel, "$8"},
		{classifyEvalModel, "0"},
		{classifyEvalModel, "-1"},
		{classifyEvalModel, "NaN"},
		{classifyEvalModel, "Inf"},
	} {
		if _, err := evalBudget(c.model, c.max); err == nil {
			t.Errorf("evalBudget(%q, %q) = nil error", c.model, c.max)
		}
	}
}

func TestWithoutCase(t *testing.T) {
	var all []ScoringExample
	for i := range MaxScoringExamples + 2 {
		all = append(all, ScoringExample{Title: fmt.Sprintf("t%d", i), Content: fmt.Sprintf("body %d", i), Label: "up"})
	}
	titles := func(ex []ScoringExample) string {
		var s []string
		for _, e := range ex {
			s = append(s, e.Title)
		}
		return strings.Join(s, ",")
	}
	sc := ScoringContext{Examples: all}

	// The case is example 2 with different whitespace: dropped, and the
	// 7th example does not move up into the prompt.
	got := withoutCase(sc, evalCase{Title: " t2 ", Content: "body\n\n  2"})
	if titles(got.Examples) != "t0,t1,t3,t4,t5" {
		t.Fatalf("examples = %s", titles(got.Examples))
	}
	if len(sc.Examples) != MaxScoringExamples+2 || sc.Examples[2].Title != "t2" {
		t.Fatal("input context was changed")
	}

	// No match: production's first six, unchanged.
	got = withoutCase(sc, evalCase{Title: "t2", Content: "other"})
	if titles(got.Examples) != "t0,t1,t2,t3,t4,t5" {
		t.Fatalf("no match: examples = %s", titles(got.Examples))
	}
}

func TestLoadEvalSet(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("contexts.json", `{"c1":{"Product":"Uptime","Examples":[{"Title":"a","Content":"b","Label":"up"}]}}`)
	write("cases.jsonl", `{"id":"x1","workspace":"ws_01","context":"c1","title":"T","content":"C","label":"lead","source":"feedback"}

{"id":"x2","workspace":"ws_02","context":"c1","label":"not_lead","source":"review"}
`)
	cases, contexts := loadEvalSet(t, dir)
	if len(cases) != 2 || cases[0].ID != "x1" || cases[0].Title != "T" || cases[1].Label != "not_lead" {
		t.Fatalf("cases = %+v", cases)
	}
	if c := contexts["c1"]; c.Product != "Uptime" || len(c.Examples) != 1 || c.Examples[0].Label != "up" {
		t.Fatalf("contexts = %+v", contexts)
	}
}
