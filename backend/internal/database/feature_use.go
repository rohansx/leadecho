package database

import (
	"context"
	"time"
)

// Feature use: when a workspace last opened a page whose background job
// spends model calls or paid data, so the job can skip workspaces nobody
// looks at. Stored under workspaces.settings.feature_use.<feature>.
const (
	FeatureDemand      = "demand"
	FeatureCompetitors = "competitors"
)

// MarkFeatureUsed records "used now" for a feature. It writes at most once an
// hour per feature, so reloading a page costs a read, not a write.
func (q *Queries) MarkFeatureUsed(ctx context.Context, workspaceID, feature string) error {
	_, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = COALESCE(settings, '{}'::jsonb) || jsonb_build_object('feature_use',
        COALESCE(settings->'feature_use', '{}'::jsonb) || jsonb_build_object($2::text, now()))
WHERE id = $1
  AND COALESCE((settings->'feature_use'->>$2::text)::timestamptz, 'epoch'::timestamptz) < now() - interval '1 hour'`,
		workspaceID, feature)
	return err
}

// FeatureLastUsed returns when a feature was last used (zero when never) and
// when the workspace was created.
func (q *Queries) FeatureLastUsed(ctx context.Context, workspaceID, feature string) (lastUsed, created time.Time, err error) {
	var at *time.Time
	err = q.db.QueryRow(ctx,
		`SELECT (settings->'feature_use'->>$2::text)::timestamptz, created_at FROM workspaces WHERE id = $1`,
		workspaceID, feature).Scan(&at, &created)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if at != nil {
		lastUsed = *at
	}
	return lastUsed, created, nil
}
