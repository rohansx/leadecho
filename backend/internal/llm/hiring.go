package llm

import (
	"context"

	"leadecho/internal/ai"
)

// PlanHiringSignals decides whether job postings signal a need for the
// workspace's product. Routed like product analysis (same tier and keys).
func (r *Router) PlanHiringSignals(ctx context.Context, workspaceID, product, overview, audience string) (*ai.HiringPlan, error) {
	var out *ai.HiringPlan
	err := r.withChatProvider(ctx, workspaceID, TaskAnalyzeProduct, func(p ai.Provider) error {
		res, err := ai.PlanHiringSignals(ctx, p, product, overview, audience)
		out = res
		return err
	})
	return out, err
}
