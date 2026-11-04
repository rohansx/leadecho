package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// callAnthropic sends a chat through the official Anthropic SDK (Messages API).
// System messages become the top-level system prompt; temperature is not sent
// because current Claude models reject sampling parameters.
//
// A refusal is returned as an error so the LLM router's existing fallback
// targets (other providers) take over.
func callAnthropic(ctx context.Context, p Provider, messages []chatMessage) (string, error) {
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

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.Model),
		MaxTokens: 16000,
		System:    system,
		Messages:  msgs,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("anthropic: request refused (%s)", resp.StopDetails.Category)
	}

	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("anthropic: empty response (stop_reason=%s)", resp.StopReason)
	}
	return strings.TrimSpace(out.String()), nil
}
