package ai

import "testing"

func TestClassifySystemSplitsForAnthropicOnly(t *testing.T) {
	sc := ScoringContext{Product: "- Acme: CRM", Buyers: "Agency owners"}
	full := buildClassifyPrompt(sc)

	a := classifySystem(Provider{Name: "anthropic"}, sc)
	if len(a) != 2 || a[0].Content != classifyBasePrompt || a[0].Content+a[1].Content != full {
		t.Fatalf("anthropic: want shared block + listener block adding up to the full prompt, got %d blocks", len(a))
	}
	if o := classifySystem(Provider{Name: "openai"}, sc); len(o) != 1 || o[0].Content != full {
		t.Fatalf("other providers get one system message, got %d", len(o))
	}
	if g := classifySystem(Provider{Name: "anthropic"}, ScoringContext{}); len(g) != 1 {
		t.Fatalf("no listener context: want one block, got %d", len(g))
	}
}

func TestNormalizeNonBuyerIsNotALead(t *testing.T) {
	got := NormalizeClassifyResult(ClassifyResult{
		IntentTags: []string{"buying_intent", "pain_point"}, RelevanceScore: 8.5,
		ConversionProbability: 0.7, AuthorRole: " Non_Buyer ",
	}, "LeetCode interviews are broken")
	if got.RelevanceScore != maxNonBuyerScore || got.ConversionProbability > 0.15 {
		t.Fatalf("non-buyer not capped: score=%v conv=%v", got.RelevanceScore, got.ConversionProbability)
	}
	if got.Intent == "buy_signal" {
		t.Fatalf("non-buyer kept a lead intent: %+v", got)
	}
	for _, role := range []string{"buyer", "", "nonsense"} {
		r := NormalizeClassifyResult(ClassifyResult{IntentTags: []string{"buying_intent"}, RelevanceScore: 8.5, AuthorRole: role}, "x")
		if r.RelevanceScore != 8.5 || r.Intent != "buy_signal" {
			t.Fatalf("role %q should not be capped: %+v", role, r)
		}
		if role != "buyer" && r.AuthorRole != "unknown" {
			t.Fatalf("role %q should normalize to unknown, got %q", role, r.AuthorRole)
		}
	}
}
