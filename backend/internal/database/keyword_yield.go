package database

import (
	"context"
	"time"
)

// KeywordYield is what one keyword brought in on one platform.
type KeywordYield struct {
	KeywordID, Term, Platform          string
	Posts, Judged, Passed, Leads, Asks int
}

// ListKeywordYield counts, per keyword and platform, the posts stored since
// `since`: all of them, those the scorer reached a verdict on, those the first
// (noise) filter kept, those scored as leads (6 or more) and those that state
// a need. Keywords with no posts in
// the window have no row. Posts stored for a brand keyword without naming the
// brand are left out: they are no longer stored at all, and a keyword that
// found no mention of the brand found nothing. Hand-written (no sqlc codegen
// needed).
func (q *Queries) ListKeywordYield(ctx context.Context, workspaceID string, since time.Time) ([]KeywordYield, error) {
	rows, err := q.db.Query(ctx, `
SELECT k.id::text, k.term, m.platform::text,
       COUNT(*)::int,
       COUNT(*) FILTER (WHERE COALESCE(m.scoring_metadata->>'stage', '') NOT IN ('', 'stage3_classification_error', 'stage3_refused'))::int,
       COUNT(*) FILTER (WHERE m.scoring_metadata->>'stage' LIKE 'stage2%' OR m.scoring_metadata->>'stage' LIKE 'stage3%')::int,
       COUNT(*) FILTER (WHERE m.relevance_score >= 6.0)::int,
       COUNT(*) FILTER (WHERE COALESCE(m.scoring_metadata->>'ask', '') <> '')::int
FROM mentions m
JOIN keywords k ON k.id = m.keyword_id
WHERE m.workspace_id = $1 AND m.created_at >= $2
  AND COALESCE(m.scoring_metadata->>'stage', '') <> 'stage0_brand_not_named'
GROUP BY k.id, k.term, m.platform`, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeywordYield
	for rows.Next() {
		var y KeywordYield
		if err := rows.Scan(&y.KeywordID, &y.Term, &y.Platform, &y.Posts, &y.Judged, &y.Passed, &y.Leads, &y.Asks); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, rows.Err()
}
