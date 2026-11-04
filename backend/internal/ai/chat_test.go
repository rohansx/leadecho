package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatReturnsUsage(t *testing.T) {
	tests := []struct {
		name      string
		usage     map[string]int
		reply     string
		msgs      []ChatMessage
		wantUsage Usage
	}{
		{
			name:      "provider usage is passed through",
			usage:     map[string]int{"prompt_tokens": 120, "completion_tokens": 30, "total_tokens": 150},
			reply:     `{"answer":"hi"}`,
			msgs:      []ChatMessage{{Role: "system", Content: "s"}, {Role: "user", Content: "q"}},
			wantUsage: Usage{PromptTokens: 120, CompletionTokens: 30, TotalTokens: 150},
		},
		{
			name:      "missing usage is estimated from text length",
			reply:     "12345678",                                                 // 8 chars -> 2 tokens
			msgs:      []ChatMessage{{Role: "user", Content: "1234567890123456"}}, // 16 chars -> 4
			wantUsage: Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tc.reply}}}}
				if tc.usage != nil {
					body["usage"] = tc.usage
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer srv.Close()
			got, u, err := Chat(context.Background(), Provider{Name: "openai", BaseURL: srv.URL, Model: "m"}, tc.msgs)
			if err != nil || got != tc.reply {
				t.Fatalf("got %q err %v", got, err)
			}
			if u != tc.wantUsage {
				t.Fatalf("usage = %+v, want %+v", u, tc.wantUsage)
			}
		})
	}
}
