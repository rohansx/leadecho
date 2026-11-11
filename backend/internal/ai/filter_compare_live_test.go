//go:build live

// Compares a cheaper model's noise-filter decisions with decisions already
// made by the current model on real posts. Run:
//
//	ANTHROPIC_API_KEY=... FILTER_SAMPLE=/path/sample.json FILTER_MODEL=claude-haiku-5-5 \
//	  go test -tags live -run TestFilterModelAgreement -v ./internal/ai/
//
// sample.json: [{"platform","title","content","sonnet_pass":bool,"score":number}, ...]
package ai

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func TestFilterModelAgreement(t *testing.T) {
	key, path, model := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("FILTER_SAMPLE"), os.Getenv("FILTER_MODEL")
	if key == "" || path == "" || model == "" {
		t.Skip("ANTHROPIC_API_KEY, FILTER_SAMPLE and FILTER_MODEL are required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var posts []struct {
		Platform, Title, Content string
		SonnetPass               bool    `json:"sonnet_pass"`
		Score                    float64 `json:"score"`
	}
	if err := json.Unmarshal(raw, &posts); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var usage Usage
	p := Provider{Name: "anthropic", APIKey: key, Model: model, UsageSink: &usage}
	got := make([]*bool, len(posts))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range posts {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			fr, err := FilterMention(ctx, p, posts[i].Title, posts[i].Content, posts[i].Platform)
			if err == nil {
				got[i] = &fr.Pass
			}
		}(i)
	}
	wg.Wait()

	var agree, failed, wronglyDropped, extraPassed, leadsDropped int
	for i, post := range posts {
		switch {
		case got[i] == nil:
			failed++
		case *got[i] == post.SonnetPass:
			agree++
		case post.SonnetPass: // the current model kept it, the candidate drops it
			wronglyDropped++
			if post.Score >= 6 {
				leadsDropped++
			}
			t.Logf("DROPPED (score %.1f): %.110s", post.Score, post.Title+" | "+post.Content)
		default:
			extraPassed++
		}
	}
	t.Logf("model=%s posts=%d agree=%d dropped_that_sonnet_kept=%d passed_that_sonnet_dropped=%d failed=%d leads_dropped=%d",
		model, len(posts), agree, wronglyDropped, extraPassed, failed, leadsDropped)
	t.Logf("tokens in=%d out=%d cache_read=%d", usage.PromptTokens, usage.CompletionTokens, usage.CacheReadTokens)
}
