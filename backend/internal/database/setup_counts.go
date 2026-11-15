package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// SetupCounts is where a workspace stands on the way to its first lead.
type SetupCounts struct {
	Posts        int       // posts collected (company signals are not posts)
	Unscored     int       // of those, not yet scored (as ListUnclassifiedMentions sees them)
	PostLeads    int       // posts that are leads by the Leads-tab rule
	CompanyLeads int       // company signals with a confirmed contact (the same rule: unconfirmed ones score below it)
	OldestLead   time.Time // when the first of those leads was stored; zero when there is none
}

// CountSetup counts a workspace's posts and leads. Hand-written (no sqlc
// codegen needed).
func (q *Queries) CountSetup(ctx context.Context, workspaceID string) (SetupCounts, error) {
	var c SetupCounts
	var oldest pgtype.Timestamptz
	err := q.db.QueryRow(ctx, `
SELECT COUNT(*) FILTER (WHERE NOT company)::int,
       COUNT(*) FILTER (WHERE NOT company AND unscored)::int,
       COUNT(*) FILTER (WHERE NOT company AND lead)::int,
       COUNT(*) FILTER (WHERE company AND lead)::int,
       MIN(created_at) FILTER (WHERE lead)
FROM (
	SELECT created_at,
	       COALESCE(platform_metadata->>'signal', '') <> '' AS company,
	       COALESCE(`+leadIntentPredicate()+`, false) AS lead,
	       (intent IS NULL AND NOT (COALESCE(scoring_metadata ? 'stage', false) AND scoring_metadata->>'stage' <> 'stage3_classification_error')) AS unscored
	FROM mentions WHERE workspace_id = $1
) m`, workspaceID).Scan(&c.Posts, &c.Unscored, &c.PostLeads, &c.CompanyLeads, &oldest)
	if oldest.Valid {
		c.OldestLead = oldest.Time
	}
	return c, err
}
