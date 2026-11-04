package llm

import "testing"

// With only an Anthropic system key and no pinned models, cheap tasks should
// run on Haiku and strong tasks (drafting, product analysis) on Opus.
func TestAnthropicTierDefaults(t *testing.T) {
	r := &Router{system: SystemKeys{AnthropicAPIKey: "k"}}
	cfg := DefaultConfig()

	cheap := r.targetsForTask(TaskClassify, cfg, nil)
	strong := r.targetsForTask(TaskDraftReply, cfg, nil)
	if len(cheap) == 0 || len(strong) == 0 {
		t.Fatalf("no targets: cheap=%v strong=%v", cheap, strong)
	}
	if cheap[0].Provider != "anthropic" || cheap[0].Model != "" {
		t.Errorf("cheap: want anthropic with provider default model, got %+v", cheap[0])
	}
	if strong[0].Provider != "anthropic" || strong[0].Model != "claude-opus-5-5" {
		t.Errorf("strong: want anthropic/claude-opus-5-5, got %+v", strong[0])
	}
	if p := providerForTarget("anthropic", "k", cheap[0], SystemKeys{}); p.Model != "claude-haiku-5-5" {
		t.Errorf("cheap resolves to %q, want claude-haiku-5-5", p.Model)
	}
}
