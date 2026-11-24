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
// EVAL_LIMIT runs only the first N cases. EVAL_SOURCES (e.g.
// agent_blind,human) keeps only those label sources. EVAL_SAMPLE=N scores a
// fixed sample of about N cases stratified by workspace and label;
// EVAL_VARIANT (base, product, buyers, looks, rules, full, story) cuts the
// workspace prompt down for an ablation, or adds the candidate storyRule (see
// ablate, classifyVariant). The report is written next to the set as
// report-<model>[-<variant>]-<time>.json.
package ai

import (
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
	maxUSD, err := evalBudget(model, envOr("EVAL_MAX_USD", "8"))
	if err != nil {
		t.Fatal(err)
	}
	cases, contexts := loadEvalSet(t, dir)
	if src := os.Getenv("EVAL_SOURCES"); src != "" {
		keep := map[string]bool{}
		for _, s := range strings.Split(src, ",") {
			keep[strings.TrimSpace(s)] = true
		}
		var kept []evalCase
		for _, c := range cases {
			if keep[c.Source] {
				kept = append(kept, c)
			}
		}
		cases = kept
	}
	if n, _ := strconv.Atoi(os.Getenv("EVAL_LIMIT")); n > 0 && n < len(cases) {
		cases = cases[:n]
	}
	if n, _ := strconv.Atoi(os.Getenv("EVAL_SAMPLE")); n > 0 {
		var ids, strata []string
		for _, c := range cases {
			ids, strata = append(ids, c.ID), append(strata, c.Workspace+"/"+c.Label)
		}
		var picked []evalCase
		for _, i := range stratifiedSample(ids, strata, n) {
			picked = append(picked, cases[i])
		}
		cases = picked
	}
	variant := os.Getenv("EVAL_VARIANT")
	t.Logf("model %s variant %q cases %d cap $%.2f", model, variant, len(cases), maxUSD)

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
		sc, err := ablate(sc, variant)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			var u Usage
			p := Provider{Name: "anthropic", APIKey: key, Model: model, UsageSink: &u}
			var cr *ClassifyResult
			var err error
			for attempt := 0; attempt < 3; attempt++ { // transient network errors (DNS EAI_AGAIN); u sums every attempt
				if cr, err = classifyVariant(ctx, p, c.Title, c.Content, c.Platform, withoutCase(sc, c), variant); err == nil {
					break
				}
				time.Sleep(time.Duration(attempt+1) * 3 * time.Second)
			}
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
	report["model"], report["variant"], report["dir"], report["ran_at"] = model, variant, dir, time.Now().UTC().Format(time.RFC3339)
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

	name := model
	if variant != "" {
		name += "-" + variant
	}
	out := filepath.Join(dir, fmt.Sprintf("report-%s-%s.json", name, time.Now().UTC().Format("20060102T150405Z")))
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
