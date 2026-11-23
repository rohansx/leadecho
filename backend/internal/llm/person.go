package llm

import (
	"context"
	"errors"

	"leadecho/internal/ai"
)

// ResearchPerson runs the identity research agent on Claude Sonnet 5.5
// (Claude's web search and fetch are needed, so only Anthropic targets run
// it). One paid run per call: a fallback target never repeats it.
func (r *Router) ResearchPerson(ctx context.Context, workspaceID string, in ai.PersonResearch, tools ai.SandboxTools) (*ai.PersonFindings, error) {
	var out *ai.PersonFindings
	var ran bool
	var ranErr error
	err := r.withChatProvider(ctx, workspaceID, TaskDraftReply, func(p ai.Provider) error {
		if ran {
			return ranErr
		}
		if p.Name != "anthropic" {
			return errors.New("person research is temporarily unavailable on this provider: it needs Claude's web tools")
		}
		p.Model = ai.PersonResearchModel
		res, _, err := ai.ResearchPerson(ctx, p, in, tools)
		ran, ranErr, out = true, err, res
		return err
	})
	return out, err
}
