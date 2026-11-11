package database

import (
	"context"
	"errors"
	"time"

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

// ClaimSettingsRun atomically records "ran now" under settings.<key>.last_run
// and reports whether this caller won: it returns false when another process
// (or an earlier tick) ran within the interval. Other fields under the key
// are kept.
func (q *Queries) ClaimSettingsRun(ctx context.Context, workspaceID, key string, interval time.Duration) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = COALESCE(settings, '{}'::jsonb) || jsonb_build_object($2::text,
        COALESCE(settings->$2::text, '{}'::jsonb) || jsonb_build_object('last_run', now()))
WHERE id = $1
  AND COALESCE((settings->$2::text->>'last_run')::timestamptz, 'epoch'::timestamptz) < now() - make_interval(secs => $3)`,
		workspaceID, key, interval.Seconds())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
