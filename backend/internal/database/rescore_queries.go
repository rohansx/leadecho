package database

import (
	"context"
	"time"
)

// RescoreMention is a post to score again with the current rules.
type RescoreMention struct {
	ID, Platform, Keyword, Title, URL, Author, Content string
}

// ListMentionsToRescore returns posts found since `since` that were scored
// under rules older than `rubric`, newest first. Left alone: posts nobody
// scored yet (the backfill takes those), company signals (hiring, prospects),
// posts the customer already acted on (any status but "new") and posts that
// are already leads. Hand-written (no sqlc codegen needed).
func (q *Queries) ListMentionsToRescore(ctx context.Context, workspaceID string, since time.Time, rubric int, limit int32) ([]RescoreMention, error) {
	rows, err := q.db.Query(ctx, `
SELECT m.id::text, m.platform::text, COALESCE(k.term, ''), COALESCE(m.title, ''), m.url,
       COALESCE(m.author_username, ''), m.content
FROM mentions m
LEFT JOIN keywords k ON k.id = m.keyword_id
WHERE m.workspace_id = $1 AND m.created_at >= $2
  AND m.status = 'new'
  AND m.scoring_metadata ? 'stage'
  AND m.scoring_metadata->>'stage' NOT LIKE 'stage0%'
  AND COALESCE((m.scoring_metadata->>'rubric')::int, 0) < $3
  AND COALESCE(m.platform_metadata->>'signal', '') = ''
  AND NOT EXISTS (SELECT 1 FROM leads l WHERE l.mention_id = m.id)
ORDER BY m.created_at DESC
LIMIT $4`, workspaceID, since, rubric, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RescoreMention
	for rows.Next() {
		var r RescoreMention
		if err := rows.Scan(&r.ID, &r.Platform, &r.Keyword, &r.Title, &r.URL, &r.Author, &r.Content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CompanyLead is a lead found as a company (a hiring signal or a prospect)
// that is still waiting in the inbox.
type CompanyLead struct {
	ID, Signal, Company, JobTitle string
	NotABuyer                     string // the stored research's verdict, "" when it had none
}

// ListCompanyLeadsToRescreen returns the company leads found since `since`
// that the customer has not acted on (status "new"), newest first.
func (q *Queries) ListCompanyLeadsToRescreen(ctx context.Context, workspaceID string, since time.Time) ([]CompanyLead, error) {
	rows, err := q.db.Query(ctx, `
SELECT id::text, platform_metadata->>'signal', COALESCE(platform_metadata->>'company', ''),
       COALESCE(platform_metadata->>'job_title', ''), COALESCE(platform_metadata->'research'->>'not_a_buyer', '')
FROM mentions
WHERE workspace_id = $1 AND created_at >= $2
  AND status = 'new'
  AND COALESCE(platform_metadata->>'signal', '') <> ''
ORDER BY created_at DESC`, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CompanyLead
	for rows.Next() {
		var l CompanyLead
		if err := rows.Scan(&l.ID, &l.Signal, &l.Company, &l.JobTitle, &l.NotABuyer); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ArchiveRescreenedLead archives a lead that failed a re-screen and stores
// the record (why, when) under scoring_metadata.rescreen. Only a
// lead still "new" is touched; it reports whether this one was.
func (q *Queries) ArchiveRescreenedLead(ctx context.Context, workspaceID, mentionID string, record []byte) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE mentions
SET status = 'archived', updated_at = now(),
    scoring_metadata = COALESCE(scoring_metadata, '{}'::jsonb) || jsonb_build_object('rescreen', $3::jsonb)
WHERE id = $1 AND workspace_id = $2 AND status = 'new'`, mentionID, workspaceID, string(record))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
