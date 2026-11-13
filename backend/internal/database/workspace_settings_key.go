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

// SwapWorkspaceSettingsKey replaces one top-level key of workspaces.settings
// only if it still holds `old` (nil = the key is absent), and reports whether
// it did. It is how a read-modify-write of that key avoids overwriting a
// change made in between. Hand-written (no sqlc codegen needed).
func (q *Queries) SwapWorkspaceSettingsKey(ctx context.Context, workspaceID, key string, old, value []byte) (bool, error) {
	var was *string
	if len(old) > 0 {
		s := string(old)
		was = &s
	}
	tag, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY[$2::text], $4::jsonb, true)
WHERE id = $1 AND (settings->$2::text) IS NOT DISTINCT FROM $3::jsonb`,
		workspaceID, key, was, string(value))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
