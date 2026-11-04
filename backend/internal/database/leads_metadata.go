package database

import "context"

// MergeLeadMetadata shallow-merges patch (a JSON object) into leads.metadata.
// Hand-written: lives outside sqlc so it needs no codegen.
func (q *Queries) MergeLeadMetadata(ctx context.Context, workspaceID, leadID string, patch []byte) error {
	_, err := q.db.Exec(ctx,
		`UPDATE leads SET metadata = metadata || $3::jsonb WHERE id = $1 AND workspace_id = $2`,
		leadID, workspaceID, patch)
	return err
}
