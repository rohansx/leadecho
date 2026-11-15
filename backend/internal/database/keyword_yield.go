package database

import (
	"context"
	"time"
)

// KeywordYield is what one keyword brought in on one platform.
type KeywordYield struct {
	KeywordID, Term, Platform  string
	Posts, Passed, Leads, Asks int
}

// ListKeywordYield counts, per keyword and platform, the posts stored since
// `since`: all of them, those the first (noise) filter kept, those scored as
// leads (6 or more) and those that state a need. Keywords with no posts in
// the window have no row. Hand-written (no sqlc codegen needed).
func (q *Queries) ListKeywordYield(ctx context.Context, workspaceID string, since time.Time) ([]KeywordYield, error) {
	rows, err := q.db.Query(ctx, `
SELECT k.id::text, k.term, m.platform::text,
       COUNT(*)::int,
       COUNT(*) FILTER (WHERE m.scoring_metadata->>'stage' LIKE 'stage2%' OR m.scoring_metadata->>'stage' LIKE 'stage3%')::int,
       COUNT(*) FILTER (WHERE m.relevance_score >= 6.0)::int,
       COUNT(*) FILTER (WHERE COALESCE(m.scoring_metadata->>'ask', '') <> '')::int
FROM mentions m
JOIN keywords k ON k.id = m.keyword_id
WHERE m.workspace_id = $1 AND m.created_at >= $2
GROUP BY k.id, k.term, m.platform`, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeywordYield
	for rows.Next() {
		var y KeywordYield
		if err := rows.Scan(&y.KeywordID, &y.Term, &y.Platform, &y.Posts, &y.Passed, &y.Leads, &y.Asks); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, rows.Err()
}
