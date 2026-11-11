package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// maxCacheBreakpoints is the API's limit on cache_control markers per request.
const maxCacheBreakpoints = 4

// callAnthropic sends a chat through the official Anthropic SDK (Messages API).
// System messages become the top-level system prompt; temperature is not sent
// because current Claude models reject sampling parameters.
//
// A refusal is returned as an error so the LLM router's existing fallback
// targets (other providers) take over.
func callAnthropic(ctx context.Context, p Provider, messages []chatMessage) (string, Usage, error) {
	client := anthropic.NewClient(option.WithAPIKey(p.APIKey))

	var system []anthropic.TextBlockParam
	var msgs []anthropic.MessageParam
	for _, m := range messages {
		switch m.Role {
		case "system":
			system = append(system, anthropic.TextBlockParam{Text: m.Content})
		case "assistant":
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		default:
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}

	// Cache the instructions: they repeat on every filter/classify call, and
	// the post (the part that varies) comes after them. Each system block is a
	// breakpoint, so a caller can send the part shared by every workspace
	// first and the per-workspace part second, and both get reused.
	// Silently a no-op below the model's minimum prefix (512 on Sonnet 5.5).
	for i := max(0, len(system)-maxCacheBreakpoints); i < len(system); i++ {
		system[i].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.Model),
		MaxTokens: 16000,
		System:    system,
		Messages:  msgs,
	}
	if p.Effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(p.Effort)}
	}
	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", Usage{}, fmt.Errorf("anthropic: request refused (%s)", resp.StopDetails.Category)
	}

	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	if out.Len() == 0 {
		return "", Usage{}, fmt.Errorf("anthropic: empty response (stop_reason=%s)", resp.StopReason)
	}
	in, outTok := int32(resp.Usage.InputTokens), int32(resp.Usage.OutputTokens)
	read, write := int32(resp.Usage.CacheReadInputTokens), int32(resp.Usage.CacheCreationInputTokens)
	return strings.TrimSpace(out.String()), Usage{
		PromptTokens: in, CompletionTokens: outTok, TotalTokens: in + read + write + outTok,
		CacheReadTokens: read, CacheWriteTokens: write,
	}, nil
}
