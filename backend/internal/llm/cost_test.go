package llm

import (
	"testing"

	"leadecho/internal/ai"
)

func TestEstimateCostUSD(t *testing.T) {
	u := ai.Usage{PromptTokens: 1_000_000, CompletionTokens: 100_000}
	cases := map[string]string{
		"claude-sonnet-5-5":           "3.000000", // $2 in + $10 out per 1M
		"claude-opus-5-5":             "6.000000", // $4 + $20
		"claude-haiku-5-5":            "0.150000", // $0.10 + $0.50
		"anthropic/claude-sonnet-5.5": "3.000000", // OpenRouter id
		"some-unknown-model":          "0",
	}
	for model, want := range cases {
		if got := estimateCostUSD(model, u); got != want {
			t.Errorf("estimateCostUSD(%q) = %s, want %s", model, got, want)
		}
	}
	if got := estimateCostUSD("claude-sonnet-5-5", ai.Usage{}); got != "0" {
		t.Errorf("no tokens = %s, want 0", got)
	}
	// Prompt caching: reads bill at 0.1x input, 5-minute writes at 1.25x.
	cached := ai.Usage{PromptTokens: 100_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 100_000, CompletionTokens: 10_000}
	// sonnet: 0.1M*$2 + 1M*$0.20 + 0.1M*$2.50 + 0.01M*$10 = 0.20+0.20+0.25+0.10
	if got := estimateCostUSD("claude-sonnet-5-5", cached); got != "0.750000" {
		t.Errorf("cached usage = %s, want 0.750000", got)
	}
}
