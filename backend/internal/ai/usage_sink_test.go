package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Task helpers (FilterMention, ClassifyIntent, …) used to drop the provider's
// token usage, so llm_usage_events recorded 0 tokens. A UsageSink on the
// Provider collects it for every call, and accumulates across calls.
func TestUsageSinkCollectsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"pass\": true, \"reason\": \"ok\"}"}}],
			"usage":{"prompt_tokens":600,"completion_tokens":40,"total_tokens":640}}`))
	}))
	defer srv.Close()

	var sink Usage
	p := Provider{Name: "openai", APIKey: "k", BaseURL: srv.URL, Model: "m", UsageSink: &sink}
	if _, err := FilterMention(context.Background(), p, "t", "c", "reddit"); err != nil {
		t.Fatal(err)
	}
	if sink.PromptTokens != 600 || sink.CompletionTokens != 40 || sink.TotalTokens != 640 {
		t.Fatalf("sink after one call = %+v", sink)
	}
	if _, err := FilterMention(context.Background(), p, "t", "c", "reddit"); err != nil {
		t.Fatal(err)
	}
	if sink.TotalTokens != 1280 {
		t.Fatalf("sink after two calls = %+v, want accumulated 1280", sink)
	}
	// No sink: still works.
	p.UsageSink = nil
	if _, err := FilterMention(context.Background(), p, "t", "c", "reddit"); err != nil {
		t.Fatal(err)
	}
}
