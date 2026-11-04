package database

import "context"

// SetWorkspaceSettingsKey replaces one top-level key of workspaces.settings
// without touching the others. Hand-written (no sqlc codegen needed).
func (q *Queries) SetWorkspaceSettingsKey(ctx context.Context, workspaceID, key string, value []byte) error {
	_, err := q.db.Exec(ctx,
		`UPDATE workspaces SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY[$2::text], $3::jsonb, true) WHERE id = $1`,
		workspaceID, key, value)
	return err
}
