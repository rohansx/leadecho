package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChat serves one OpenAI-compatible completion and records the prompts.
func fakeChat(t *testing.T, reply string) (Provider, *[]chatMessage) {
	t.Helper()
	var got []chatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		got = req.Messages
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": reply}}},
		})
	}))
	t.Cleanup(srv.Close)
	return Provider{Name: "openai", APIKey: "k", BaseURL: srv.URL, Model: "m"}, &got
}

func TestClassifyIntentPromptIncludesListenerContext(t *testing.T) {
	p, msgs := fakeChat(t, `{"intent_tags":["buying_intent"],"sentiment":"negative","relevance_score":8,"post_type":"question"}`)
	sc := ScoringContext{
		Product:  "- Acme: uptime monitoring",
		LooksFor: "teams outgrowing Pingdom",
		Skips:    "job posts, students",
		Examples: []ScoringExample{
			{Title: "Pingdom too pricey", Content: "need cheaper uptime alerts", Label: "up", Note: "perfect lead"},
			{Title: "Hiring SRE", Content: "we are hiring", Label: "down"},
		},
	}
	res, err := ClassifyIntent(context.Background(), p, "t", "c", "reddit", sc)
	if err != nil {
		t.Fatal(err)
	}
	sys := (*msgs)[0].Content
	for _, want := range []string{
		"uptime monitoring", "LOOKS FOR", "teams outgrowing Pingdom", "SKIPS", "job posts, students",
		"matches_skips", "GOOD LEAD", "Pingdom too pricey", "perfect lead", "NOT A LEAD", "Hiring SRE",
		"seeking_alternative", "promotional", "thought_leadership",
		"must NEVER be tagged buying_intent or seeking_alternative",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	if res.Intent != "buy_signal" || res.Sentiment != "negative" || len(res.IntentTags) != 1 {
		t.Fatalf("result not normalized: %+v", res)
	}
}

func TestClassifyIntentPromptOmitsEmptySections(t *testing.T) {
	p, msgs := fakeChat(t, `{"intent":"general"}`)
	if _, err := ClassifyIntent(context.Background(), p, "t", "c", "hn", ScoringContext{}); err != nil {
		t.Fatal(err)
	}
	sys := (*msgs)[0].Content
	for _, absent := range []string{"LOOKS FOR", "SKIPS:", "GOOD LEAD", "THE PRODUCT you are scoring for"} {
		if strings.Contains(sys, absent) {
			t.Errorf("prompt should not contain %q when context is empty", absent)
		}
	}
}

func TestClassifyIntentAnnouncementGuard(t *testing.T) {
	p, _ := fakeChat(t, `{"intent":"buy_signal","intent_tags":["seeking_alternative"],"relevance_score":9}`)
	res, err := ClassifyIntent(context.Background(), p, "Show HN: an alternative to Sentry", "c", "hackernews", ScoringContext{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent != "general" || res.RelevanceScore > 3 {
		t.Fatalf("announcement leaked through: %+v", res)
	}
}

func TestFormatExamplesTruncatesAndCaps(t *testing.T) {
	long := strings.Repeat("x", 2000)
	var ex []ScoringExample
	for i := 0; i < 20; i++ {
		ex = append(ex, ScoringExample{Content: long, Label: "up"})
	}
	out := formatExamples(ex)
	if n := strings.Count(out, "GOOD LEAD"); n != MaxScoringExamples {
		t.Fatalf("examples = %d, want %d", n, MaxScoringExamples)
	}
	if len(out) > MaxScoringExamples*700 {
		t.Fatalf("examples not truncated: %d bytes", len(out))
	}
}
