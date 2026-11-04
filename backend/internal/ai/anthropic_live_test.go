package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live smoke test against the real Anthropic API. Skipped unless
// ANTHROPIC_API_KEY is set, so CI never spends money by accident.
func TestAnthropicLiveClassifyAndFilter(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p := DefaultProvider("anthropic", key)

	res, err := ClassifyIntent(ctx, p,
		"Looking for a tool to find leads on Reddit",
		"We're a 3-person B2B SaaS. Any recommendations for monitoring Reddit/HN for people asking about our category? Budget ~$100/mo.",
		"reddit", ScoringContext{})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	t.Logf("classify: %+v", *res)
	if res.Intent == "" || res.RelevanceScore <= 0 {
		t.Fatalf("classification looks empty: %+v", *res)
	}

	f, err := FilterMention(ctx, p, "Win a free iPhone!!!", "Click my link to claim your prize now", "reddit")
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	t.Logf("filter: %+v", *f)
}
