package llm

import (
	"context"

	"leadecho/internal/ai"
)

// SuggestKeywords and ReviewKeywords run on the strong tier, routed like
// product analysis: they are rare (a click, or once a week) and their output
// decides what the workspace spends its credits searching.

func (r *Router) SuggestKeywords(ctx context.Context, workspaceID string, req ai.KeywordSuggestRequest) ([]ai.SuggestedKeyword, error) {
	var out []ai.SuggestedKeyword
	err := r.withChatProvider(ctx, workspaceID, TaskAnalyzeProduct, func(p ai.Provider) error {
		res, err := ai.SuggestKeywords(ctx, p, req)
		out = res
		return err
	})
	return out, err
}

func (r *Router) ReviewKeywords(ctx context.Context, workspaceID string, in ai.KeywordReviewInput) (*ai.KeywordReview, error) {
	var out *ai.KeywordReview
	err := r.withChatProvider(ctx, workspaceID, TaskAnalyzeProduct, func(p ai.Provider) error {
		res, err := ai.ReviewKeywords(ctx, p, in)
		out = res
		return err
	})
	return out, err
}
