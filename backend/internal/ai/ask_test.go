package ai

import (
	"strings"
	"testing"
)

func TestClassifyPromptAsksForTheNeed(t *testing.T) {
	p := buildClassifyPrompt(ScoringContext{})
	if !strings.Contains(p, `"ask"`) || !strings.Contains(p, "what the author wants") {
		t.Fatal(`the classifier must return "ask": the author's need in one line`)
	}
}

func TestNormalizeAsk(t *testing.T) {
	got := NormalizeClassifyResult(ClassifyResult{Ask: "  How do I screen\nsenior engineers for judgment?  "}, "t")
	if got.Ask != "How do I screen senior engineers for judgment?" {
		t.Fatalf("ask = %q", got.Ask)
	}
	long := NormalizeClassifyResult(ClassifyResult{Ask: strings.Repeat("é", 500)}, "t")
	if n := len([]rune(long.Ask)); n > maxAskLen+1 {
		t.Fatalf("ask not capped: %d runes", n)
	}
	// A launch or advert is someone selling, not asking.
	promo := NormalizeClassifyResult(ClassifyResult{Ask: "Try our tool", PostType: "announcement"}, "Show HN: my tool")
	if promo.Ask != "" {
		t.Fatalf("an announcement has no ask, got %q", promo.Ask)
	}
	for _, none := range []string{"none", "N/A", "-", "  "} {
		if got := NormalizeClassifyResult(ClassifyResult{Ask: none}, "t"); got.Ask != "" {
			t.Errorf("%q should mean no ask, got %q", none, got.Ask)
		}
	}
}
