package llm

import (
	"context"
	"errors"

	"leadecho/internal/ai"
)

// TaskPersonResearch is the identity research agent (its own usage line).
const TaskPersonResearch Task = "person_research"

// ResearchPerson runs the identity research agent on Claude Sonnet 5.5
// (Claude's web search and fetch are needed, so only Anthropic targets run
// it). One paid run per call: a fallback target never repeats it.
func (r *Router) ResearchPerson(ctx context.Context, workspaceID string, in ai.PersonResearch, tools ai.SandboxTools) (*ai.PersonFindings, error) {
	var out *ai.PersonFindings
	var ran bool
	var ranErr error
	err := r.withChatProviderModel(ctx, workspaceID, TaskPersonResearch, ai.PersonResearchModel, func(p ai.Provider) (ai.Usage, error) {
		if ran {
			return ai.Usage{}, ranErr
		}
		if p.Name != "anthropic" {
			return ai.Usage{}, errors.New("person research is temporarily unavailable on this provider: it needs Claude's web tools")
		}
		res, u, err := ai.ResearchPerson(ctx, p, in, tools)
		ran, ranErr, out = true, err, res
		return u, err
	})
	return out, err
}
