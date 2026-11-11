package llm

import "testing"

// With only an Anthropic system key and no pinned models, cheap tasks
// (the noise filter, chat) run on Sonnet 5.5 at low effort and strong tasks
// (classification, drafting, product analysis) on Opus 5.5 at its default
// effort. Classification decides who is a buyer and what becomes a lead, so
// it gets the stronger model: in a blind comparison on real posts Opus was
// right more often where the two disagreed.
func TestAnthropicTierDefaults(t *testing.T) {
	r := &Router{system: SystemKeys{AnthropicAPIKey: "k"}}
	cfg := DefaultConfig()

	cheap := r.targetsForTask(TaskFilter, cfg, nil)
	if c := r.targetsForTask(TaskClassify, cfg, nil); len(c) == 0 || c[0].Model != "claude-opus-5-5" {
		t.Errorf("classification: want anthropic/claude-opus-5-5, got %+v", c)
	}
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
	if p := providerForTarget("anthropic", "k", cheap[0], SystemKeys{}); p.Model != "claude-sonnet-5-5" || p.Effort != "low" {
		t.Errorf("cheap resolves to %q effort %q, want claude-sonnet-5-5 low", p.Model, p.Effort)
	}
	if p := providerForTarget("anthropic", "k", strong[0], SystemKeys{}); p.Model != "claude-opus-5-5" || p.Effort != "" {
		t.Errorf("strong resolves to %q effort %q, want claude-opus-5-5 with the model default", p.Model, p.Effort)
	}
}
