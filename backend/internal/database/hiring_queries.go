package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Hand-written queries for the hiring-signal source (no sqlc codegen needed).

// GetWorkspaceSettingsKey returns one top-level key of workspaces.settings
// (nil when the key or workspace is missing).
func (q *Queries) GetWorkspaceSettingsKey(ctx context.Context, workspaceID, key string) ([]byte, error) {
	var v []byte
	err := q.db.QueryRow(ctx, `SELECT settings->$2::text FROM workspaces WHERE id = $1`, workspaceID, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

// MentionExists reports whether the workspace already has this platform post.
func (q *Queries) MentionExists(ctx context.Context, workspaceID, platform, platformID string) (bool, error) {
	var ok bool
	err := q.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM mentions WHERE workspace_id = $1 AND platform = $2::platform_type AND platform_id = $3)`,
		workspaceID, platform, platformID).Scan(&ok)
	return ok, err
}
