//go:build live

// Scores real posts (authors anonymised) whose shape the classifier got
// wrong, next to noise it must keep out, and checks each lands on the right
// side of the lead line. Spends a few cents. Run:
//
//	ANTHROPIC_API_KEY=... go test -tags live -run TestBuyerShapesEval -v ./internal/ai/
//
// EVAL_MODEL overrides the model (default: the one classification runs on).
package ai

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestBuyerShapesEval(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	model := os.Getenv("EVAL_MODEL")
	if model == "" {
		model = "claude-opus-5-5"
	}
	doc := loadBuyerShapes(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	p := Provider{Name: "anthropic", APIKey: key, Model: model}
	sc := ScoringContext{Product: doc.Product, Buyers: doc.Buyers}
	got := make([]*ClassifyResult, len(doc.Cases))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, c := range doc.Cases {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if cr, err := ClassifyIntent(ctx, p, c.Title, c.Content, c.Platform, sc); err == nil {
				got[i] = cr
			} else {
				t.Logf("%s: classify failed: %v", c.ID, err)
			}
		}()
	}
	wg.Wait()

	wrong := 0
	for i, c := range doc.Cases {
		cr := got[i]
		if cr == nil {
			wrong++
			t.Errorf("%s: no result", c.ID)
			continue
		}
		lead := isLead(cr)
		ok := lead == (c.Want == "lead") && cr.RelevanceScore >= c.MinScore && (!c.WantAsk || cr.Ask != "")
		mark := "ok  "
		if !ok {
			mark = "MISS"
			wrong++
			t.Errorf("%s: want %s (min score %.0f, ask %v), got score %.1f lead=%v ask=%q", c.ID, c.Want, c.MinScore, c.WantAsk, cr.RelevanceScore, lead, cr.Ask)
		}
		t.Logf("%s %-20s want=%-8s score=%.1f role=%-9s type=%-18s intent=%-18s ask=%q", mark, c.ID, c.Want, cr.RelevanceScore, cr.AuthorRole, cr.PostType, cr.Intent, cr.Ask)
	}
	t.Logf("model=%s cases=%d wrong=%d", model, len(doc.Cases), wrong)
}
