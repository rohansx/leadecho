package database

import (
	"context"
	"time"
)

// DigestHiringSignals counts the hiring signals found in a period: confirmed
// ones (a lead: score 6+) and the rest (held back with a reason).
// Hand-written (no sqlc codegen needed).
func (q *Queries) DigestHiringSignals(ctx context.Context, workspaceID string, start, end time.Time) (confirmed, held int, err error) {
	err = q.db.QueryRow(ctx, `
SELECT COUNT(*) FILTER (WHERE relevance_score >= 6.0)::int,
       COUNT(*) FILTER (WHERE relevance_score IS NULL OR relevance_score < 6.0)::int
FROM mentions
WHERE workspace_id = $1 AND created_at >= $2 AND created_at < $3
  AND platform_metadata->>'signal' = 'hiring'
  AND status NOT IN ('spam', 'archived')`, workspaceID, start, end).Scan(&confirmed, &held)
	return confirmed, held, err
}
