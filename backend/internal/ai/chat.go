package ai

import "context"

// chat: free-form multi-turn completion for the in-app assistant.

// ChatMessage is one turn sent to the model (role: system | user | assistant).
type ChatMessage = chatMessage

// Usage is the token count of one model call.
type Usage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
	// Prompt caching (Anthropic): PromptTokens is the uncached input only.
	CacheReadTokens  int32 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int32 `json:"cache_write_tokens,omitempty"`
}

// Add accumulates another call's usage.
func (u *Usage) Add(o Usage) {
	u.PromptTokens += o.PromptTokens
	u.CompletionTokens += o.CompletionTokens
	u.TotalTokens += o.TotalTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
}

// Chat sends msgs and returns the reply with its token usage. When the provider
// reports no usage it is estimated at ~4 characters per token.
func Chat(ctx context.Context, p Provider, msgs []ChatMessage) (string, Usage, error) {
	out, u, err := callChatUsage(ctx, p, msgs, 0.2)
	if err != nil {
		return "", Usage{}, err
	}
	if u.TotalTokens == 0 {
		// ponytail: chars/4 estimate; real tokenizer only if billing needs exactness
		n := 0
		for _, m := range msgs {
			n += len(m.Content)
		}
		u = Usage{PromptTokens: int32(n / 4), CompletionTokens: int32(len(out) / 4)}
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return out, u, nil
}
