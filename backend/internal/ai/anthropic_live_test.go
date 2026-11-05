//go:build live

// Live check that the cheap tier (Sonnet 5.5, effort low) is accepted and
// parses. Spends well under a cent. Run:
//
//	ANTHROPIC_API_KEY=... go test -tags live -run TestAnthropicLive -v ./internal/ai/
package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestAnthropicLive(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	p := Provider{Name: "anthropic", APIKey: key, Model: "claude-sonnet-5-5", Effort: "low"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	title := "MCP from Lucidchart has view only access to diagram instead of edit access"
	content := "We connected the Lucidchart MCP server to our agent but it can only read diagrams. Is there an alternative that lets agents edit, with proper permission controls?"

	start := time.Now()
	fr, err := FilterMention(ctx, p, title, content, "reddit")
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	t.Logf("filter pass=%v reason=%q (%s)", fr.Pass, fr.Reason, time.Since(start).Round(time.Millisecond))

	start = time.Now()
	cr, err := ClassifyIntent(ctx, p, title, content, "reddit", ScoringContext{})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	t.Logf("classify intent=%s conversion=%.2f (%s)", cr.Intent, cr.ConversionProbability, time.Since(start).Round(time.Millisecond))

	// Token tracking + prompt caching: two filters on different posts; the
	// shared instructions should be read from cache on the second.
	for i, post := range []string{"Any good alternative to Lakera Guard for agent tool calls?", "Just shipped my portfolio site, feedback welcome!"} {
		var u Usage
		p.UsageSink = &u
		if _, err := FilterMention(ctx, p, "", post, "reddit"); err != nil {
			t.Fatalf("filter %d: %v", i, err)
		}
		t.Logf("filter %d usage: in=%d cache_write=%d cache_read=%d out=%d", i, u.PromptTokens, u.CacheWriteTokens, u.CacheReadTokens, u.CompletionTokens)
		if u.TotalTokens == 0 {
			t.Errorf("filter %d recorded 0 tokens", i)
		}
	}
	sc := ScoringContext{Product: "CloakPipe: a policy proxy for AI agent tool calls (MCP). Allowlists, least-privilege, audit logs, idempotent retries."}
	for i, post := range []string{"Any good alternative to Lakera Guard for agent tool calls?", "How do you audit what an AI agent actually did in prod?"} {
		var u Usage
		p.UsageSink = &u
		if _, err := ClassifyIntent(ctx, p, "", post, "reddit", sc); err != nil {
			t.Fatalf("classify %d: %v", i, err)
		}
		t.Logf("classify %d usage: in=%d cache_write=%d cache_read=%d out=%d", i, u.PromptTokens, u.CacheWriteTokens, u.CacheReadTokens, u.CompletionTokens)
	}
}
