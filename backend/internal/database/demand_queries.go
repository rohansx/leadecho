package database

import (
	"context"
	"time"
)

// MentionAsk is one scored post that states a need (scoring_metadata.ask).
type MentionAsk struct {
	MentionID  string
	Ask        string
	AuthorRole string
	Platform   string
	URL        string
	Author     string
	At         time.Time
}

// ListMentionAsks returns the asks from posts scored since `since`, newest
// first, whose relevance is at least minRelevance. Left out: spam, the workspace's hiring signals (a company, not a
// person stating a need), and authors the scorer judged not to be buyers
// (job seekers, vendors): their needs are not demand for the product.
// Hand-written (no sqlc codegen needed).
func (q *Queries) ListMentionAsks(ctx context.Context, workspaceID string, since time.Time, minRelevance float64, limit int32) ([]MentionAsk, error) {
	rows, err := q.db.Query(ctx, `
SELECT id::text, scoring_metadata->>'ask', COALESCE(scoring_metadata->>'author_role', ''),
       platform::text, url, COALESCE(author_username, ''), COALESCE(platform_created_at, created_at)
FROM mentions
WHERE workspace_id = $1 AND created_at >= $2
  AND COALESCE(scoring_metadata->>'ask', '') <> ''
  AND status <> 'spam'
  AND COALESCE(relevance_score, 0) >= $4
  AND COALESCE(scoring_metadata->>'author_role', '') <> 'non_buyer'
  AND COALESCE(platform_metadata->>'signal', '') <> 'hiring'
ORDER BY created_at DESC
LIMIT $3`, workspaceID, since, limit, minRelevance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MentionAsk
	for rows.Next() {
		var a MentionAsk
		if err := rows.Scan(&a.MentionID, &a.Ask, &a.AuthorRole, &a.Platform, &a.URL, &a.Author, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
