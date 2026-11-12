package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ManualScanKey is the workspaces.settings key holding an operator's
// "scan now" request and its outcome. Hand-written (no sqlc codegen needed).
const ManualScanKey = "manual_scan"

// ListRequestedManualScans returns the workspaces with a manual scan waiting.
func (q *Queries) ListRequestedManualScans(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `SELECT id::text FROM workspaces WHERE settings->'manual_scan'->>'state' = 'requested'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ClaimManualScan atomically moves a waiting request to "running" and
// returns it. ok is false when there is none (or another process took it).
func (q *Queries) ClaimManualScan(ctx context.Context, workspaceID string, startedAt time.Time) (raw []byte, ok bool, err error) {
	err = q.db.QueryRow(ctx, `
UPDATE workspaces
SET settings = jsonb_set(jsonb_set(settings, '{manual_scan,state}', '"running"'::jsonb),
                         '{manual_scan,started_at}', to_jsonb($2::text))
WHERE id = $1 AND settings->'manual_scan'->>'state' = 'requested'
RETURNING settings->'manual_scan'`, workspaceID, startedAt.Format(time.RFC3339)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return raw, err == nil, err
}

// ListManualScans returns every workspace's last manual scan, by workspace id.
func (q *Queries) ListManualScans(ctx context.Context) (map[string][]byte, error) {
	rows, err := q.db.Query(ctx, `SELECT id::text, settings->'manual_scan' FROM workspaces WHERE settings ? 'manual_scan'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out[id] = raw
	}
	return out, rows.Err()
}

// RequestManualScan stores a new request unless one is already waiting or
// running and is newer than staleBefore. It reports whether it was stored.
// One statement, so two operators clicking together cannot both win.
func (q *Queries) RequestManualScan(ctx context.Context, workspaceID string, scan []byte, staleBefore time.Time) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY['manual_scan'], $2::jsonb, true)
WHERE id = $1
  AND NOT (COALESCE(settings->'manual_scan'->>'state', '') IN ('requested', 'running')
           AND COALESCE(settings->'manual_scan'->>'started_at', settings->'manual_scan'->>'requested_at', '1970-01-01T00:00:00Z')::timestamptz > $3)`,
		workspaceID, string(scan), staleBefore)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// SaveManualScan stores a manual scan's new state. With requestedAt set it
// only does so while that same request is still the one on record, so a
// finished run cannot overwrite a newer request.
func (q *Queries) SaveManualScan(ctx context.Context, workspaceID string, scan []byte, requestedAt string) error {
	_, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY['manual_scan'], $2::jsonb, true)
WHERE id = $1 AND ($3 = '' OR settings->'manual_scan'->>'requested_at' = $3)`,
		workspaceID, string(scan), requestedAt)
	return err
}
