package llm

import (
	"context"

	"leadecho/internal/ai"
)

// TaskDigestSummary routes the weekly digest paragraph (P1 #13); cheap tier by default.
const TaskDigestSummary Task = "digest_summary"

func (r *Router) SummarizeDigest(ctx context.Context, workspaceID, facts string) (string, error) {
	var out string
	err := r.withChatProvider(ctx, workspaceID, TaskDigestSummary, func(p ai.Provider) error {
		res, err := ai.SummarizeDigest(ctx, p, facts)
		out = res
		return err
	})
	return out, err
}
