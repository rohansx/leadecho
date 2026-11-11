package database

import "context"

// ActivateKeywordsUpTo switches paused keywords back on, oldest first, until
// the workspace has `limit` active ones. Used when a plan's keyword limit
// goes up: the keywords the old limit left paused start scanning.
// Returns how many were switched on.
//
// ponytail: can't tell "paused by the limit" from "paused by the user", so an
// upgrade re-enables both. Add a paused_by column if that annoys anyone.
func (q *Queries) ActivateKeywordsUpTo(ctx context.Context, workspaceID string, limit int32) (int64, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE keywords SET is_active = true, updated_at = now()
WHERE id IN (
    SELECT id FROM keywords
    WHERE workspace_id = $1 AND NOT is_active
    ORDER BY created_at, id
    LIMIT GREATEST($2 - (SELECT count(*) FROM keywords WHERE workspace_id = $1 AND is_active), 0)
)`, workspaceID, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
