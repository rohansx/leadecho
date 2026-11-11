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

// DraftOutreach writes the first message to the buyer behind a hiring signal.
func (r *Router) DraftOutreach(ctx context.Context, workspaceID string, o ai.Outreach) (string, error) {
	var out string
	err := r.withChatProvider(ctx, workspaceID, TaskDraftReply, func(p ai.Provider) error {
		res, err := ai.DraftOutreach(ctx, p, o)
		out = res
		return err
	})
	return out, err
}

// ResearchCompany searches the web for a confirmed lead's company. Routed
// like reply drafting (the strong tier); it needs Claude's web search tool,
// so targets on other providers are skipped.
func (r *Router) ResearchCompany(ctx context.Context, workspaceID string, in ai.CompanyResearch) (*ai.ResearchBrief, error) {
	var out *ai.ResearchBrief
	var ran bool
	var ranErr error
	err := r.withChatProvider(ctx, workspaceID, TaskDraftReply, func(p ai.Provider) error {
		if ran {
			// One paid run per lead: a fallback target must not repeat
			// several searches because the first run failed.
			return ranErr
		}
		res, _, err := ai.ResearchCompany(ctx, p, in)
		if p.Name == "anthropic" {
			ran, ranErr = true, err
		}
		out = res
		return err
	})
	return out, err
}
