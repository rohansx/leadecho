package ai

import (
	"context"
	"errors"
	"strings"
)

// SummarizeDigest writes the weekly digest paragraph from a plain-text fact sheet.
func SummarizeDigest(ctx context.Context, p Provider, facts string) (string, error) {
	systemPrompt := `You write the summary paragraph of a weekly social-listening digest for a B2B founder.
Use ONLY the numbers in the facts. 2-4 sentences, plain text, no markdown, no greeting.
Lead with buyers found and revenue attributed, mention competitor complaints and noisy keywords worth muting, and end with one concrete suggestion.`
	out, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Facts:\n" + facts},
	}, 0.4)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", errors.New("empty digest summary")
	}
	return out, nil
}
