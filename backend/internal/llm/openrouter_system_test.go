package llm

import "testing"

// A server-wide OPENROUTER_API_KEY alone must be enough to run chat tasks,
// and OPENROUTER_MODEL must pick the model.
func TestOpenRouterSystemKey(t *testing.T) {
	sys := SystemKeys{OpenRouterAPIKey: "k", OpenRouterModel: "anthropic/some-claude"}
	r := &Router{system: sys}
	cfg := DefaultConfig()

	targets := r.targetsForTask(TaskClassify, cfg, nil)
	if len(targets) == 0 || targets[0].Provider != "openrouter" {
		t.Fatalf("want openrouter target, got %+v", targets)
	}
	key, source, err := r.resolveKey("openrouter", cfg, nil)
	if err != nil || key != "k" || source != "env" {
		t.Fatalf("resolveKey = %q %q %v, want k env nil", key, source, err)
	}
	if p := providerForTarget("openrouter", key, targets[0], sys); p.Model != "anthropic/some-claude" {
		t.Errorf("model = %q, want OPENROUTER_MODEL override", p.Model)
	}
	if p := providerForTarget("openrouter", key, targets[0], SystemKeys{}); p.Model != "openai/gpt-4o-mini" {
		t.Errorf("model without override = %q, want built-in default", p.Model)
	}
}
