//go:build live

// Scores a labelled set of real posts from several workspaces with each
// workspace's own scoring context and reports precision, recall and the
// false-positive rate per workspace, per label source and overall. The set
// lives outside the repo (customer data); see its README.md. Run:
//
//	ANTHROPIC_API_KEY=... LEADECHO_EVAL_DIR=~/.config/workz/leadecho-evals \
//	  go test -tags live -run TestFeedbackEval -v -timeout 60m ./internal/ai/
//
// EVAL_MODEL overrides the model, EVAL_MAX_USD the spend cap (default 8),
// EVAL_LIMIT runs only the first N cases. The report is written next to the
// set as report-<model>-<time>.json.
package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

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

type evalResult struct {
	evalCase
	Got       bool    `json:"got_lead"`
	Score     float64 `json:"score"`
	Intent    string  `json:"intent"`
	PostType  string  `json:"post_type"`
	Role      string  `json:"author_role"`
	Reasoning string  `json:"reasoning"`
	Err       string  `json:"error,omitempty"`
	CostUSD   float64 `json:"cost_usd"`
	Tokens    Usage   `json:"tokens"`
}

func TestFeedbackEval(t *testing.T) {
	key, dir := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("LEADECHO_EVAL_DIR")
	if key == "" || dir == "" {
		t.Skip("ANTHROPIC_API_KEY and LEADECHO_EVAL_DIR must be set")
	}
	model := envOr("EVAL_MODEL", classifyEvalModel)
	maxUSD, _ := strconv.ParseFloat(envOr("EVAL_MAX_USD", "8"), 64)
	cases, contexts := loadEvalSet(t, dir)
	if n, _ := strconv.Atoi(os.Getenv("EVAL_LIMIT")); n > 0 && n < len(cases) {
		cases = cases[:n]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	defer cancel()
	results := make([]*evalResult, len(cases))
	var (
		mu    sync.Mutex
		spent float64
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, 4)
	for i, c := range cases {
		mu.Lock()
		over := spent > maxUSD
		mu.Unlock()
		if over {
			t.Logf("spend cap $%.2f passed after %d cases; stopping", maxUSD, i)
			break
		}
		sc, ok := contexts[c.Context]
		if !ok {
			t.Fatalf("%s: no context %q", c.ID, c.Context)
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			var u Usage
			p := Provider{Name: "anthropic", APIKey: key, Model: model, UsageSink: &u}
			cr, err := ClassifyIntent(ctx, p, c.Title, c.Content, c.Platform, withoutCase(sc, c))
			r := &evalResult{evalCase: c, Tokens: u, CostUSD: evalCostUSD(model, u)}
			if err != nil {
				r.Err = err.Error()
			} else {
				r.Got, r.Score, r.Intent, r.PostType, r.Role, r.Reasoning = isLead(cr), cr.RelevanceScore, cr.Intent, cr.PostType, cr.AuthorRole, cr.Reasoning
			}
			mu.Lock()
			spent += r.CostUSD
			results[i] = r
			mu.Unlock()
		}()
	}
	wg.Wait()

	report := summarise(results)
	report["model"], report["dir"], report["ran_at"] = model, dir, time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		if r != nil && (r.Got != (r.Label == "lead") || r.Err != "") {
			t.Logf("MISS %-10s %-7s want=%-8s score=%.1f intent=%-18s type=%-18s src=%s %q: %s%s",
				r.ID, r.Workspace, r.Label, r.Score, r.Intent, r.PostType, r.Source, clip(r.Title+" "+r.Content, 90), r.Reasoning, r.Err)
		}
	}
	for _, group := range []string{"by_workspace", "by_source"} {
		for name, m := range report[group].(map[string]*evalMetrics) {
			t.Logf("%-12s %-16s %s", group, name, fmtMetrics(m))
		}
	}
	t.Logf("overall      %-16s %s", "all", fmtMetrics(report["overall"].(*evalMetrics)))

	out := filepath.Join(dir, fmt.Sprintf("report-%s-%s.json", model, time.Now().UTC().Format("20060102T150405Z")))
	raw, _ := json.MarshalIndent(report, "", " ")
	if err := os.WriteFile(out, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("report: %s", out)
}

// summarise builds per-workspace, per-source and overall metrics.
func summarise(results []*evalResult) map[string]any {
	overall := &evalMetrics{}
	byWS, bySrc := map[string]*evalMetrics{}, map[string]*evalMetrics{}
	var done []*evalResult
	for _, r := range results {
		if r == nil {
			continue // not run (spend cap)
		}
		done = append(done, r)
		for _, m := range []*evalMetrics{overall, group(byWS, r.Workspace), group(bySrc, r.Source)} {
			m.CostUSD += r.CostUSD
			m.Tokens.Add(r.Tokens)
			if r.Err != "" {
				m.Errors++
				continue
			}
			m.add(r.Label == "lead", r.Got)
		}
	}
	overall.finish()
	for _, m := range byWS {
		m.finish()
	}
	for _, m := range bySrc {
		m.finish()
	}
	sort.Slice(done, func(i, j int) bool { return done[i].ID < done[j].ID })
	return map[string]any{"overall": overall, "by_workspace": byWS, "by_source": bySrc, "cases": done}
}

func group(m map[string]*evalMetrics, k string) *evalMetrics {
	if m[k] == nil {
		m[k] = &evalMetrics{}
	}
	return m[k]
}

// withoutCase drops a few-shot example that is the case itself, so a post
// the workspace already labelled is not scored with its own answer.
func withoutCase(sc ScoringContext, c evalCase) ScoringContext {
	key := clip(c.Title+c.Content, 200)
	var ex []ScoringExample
	for _, e := range sc.Examples {
		if clip(e.Title+e.Content, 200) != key {
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

func fmtMetrics(m *evalMetrics) string {
	return fmt.Sprintf("n=%-3d precision=%.2f recall=%.2f fpr=%.2f tp=%d fp=%d tn=%d fn=%d err=%d cost=$%.3f",
		m.N, m.Precision, m.Recall, m.FPRate, m.TP, m.FP, m.TN, m.FN, m.Errors, m.CostUSD)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
