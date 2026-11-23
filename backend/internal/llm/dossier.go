package llm

import (
	"context"

	"leadecho/internal/ai"
)

// TaskPersonDossier writes a lead author's profile (cheap tier: one short call).
const TaskPersonDossier Task = "person_dossier"

// BuildPersonDossier runs ai.BuildPersonDossier on the workspace's routed model.
func (r *Router) BuildPersonDossier(ctx context.Context, workspaceID string, in ai.DossierInput) (*ai.Dossier, error) {
	var out *ai.Dossier
	err := r.withChatProvider(ctx, workspaceID, TaskPersonDossier, func(p ai.Provider) error {
		res, err := ai.BuildPersonDossier(ctx, p, in)
		out = res
		return err
	})
	return out, err
}
