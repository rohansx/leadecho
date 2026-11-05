package llm

import (
	"context"

	"leadecho/internal/ai"
)

// GenerateListener drafts keywords + Looks-for / Skips for one onboarding goal.
// Routed like product analysis (same tier, same BYOK keys).
func (r *Router) GenerateListener(ctx context.Context, workspaceID string, lp ai.ListenerPrompt) (*ai.ListenerDraft, error) {
	var out *ai.ListenerDraft
	err := r.withChatProvider(ctx, workspaceID, TaskAnalyzeProduct, func(p ai.Provider) error {
		res, err := ai.GenerateListener(ctx, p, lp)
		out = res
		return err
	})
	return out, err
}
