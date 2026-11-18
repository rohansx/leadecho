package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Keyword review: the judge's last report lives under
// workspaces.settings.keyword_review. Hand-written (no sqlc codegen needed).

// ReviewPostRow is one post sampled for the keyword judge.
type ReviewPostRow struct {
	Platform, Keyword, Stage, Text, Reason string
	Score                                  float64
	IsLead                                 bool
}

// SampleReviewPosts returns a sample of the workspace's posts since `since`
// for the judge: up to 20 leads, 12 near misses (scored 4 to 6, not a lead),
// 12 low scores and 16 posts the noise filters dropped, newest first in each
// group. Text is the title and body, clipped.
func (q *Queries) SampleReviewPosts(ctx context.Context, workspaceID string, since time.Time) ([]ReviewPostRow, error) {
	rows, err := q.db.Query(ctx, `
WITH m AS (
  SELECT m.platform::text AS platform, COALESCE(k.term, '') AS keyword,
         COALESCE(m.scoring_metadata->>'stage', '') AS stage,
         left(regexp_replace(trim(COALESCE(m.title, '') || ' ' || COALESCE(m.content, '')), '\s+', ' ', 'g'), 420) AS text,
         left(COALESCE(m.scoring_metadata->>'reasoning', m.scoring_metadata->>'reason', ''), 220) AS reason,
         COALESCE(m.relevance_score, 0)::float8 AS score,
         EXISTS (SELECT 1 FROM leads l WHERE l.mention_id = m.id) AS is_lead,
         m.created_at
  FROM mentions m LEFT JOIN keywords k ON k.id = m.keyword_id
  WHERE m.workspace_id = $1 AND m.created_at >= $2
), g AS (
  SELECT *, CASE
      WHEN is_lead THEN 1
      WHEN stage = 'stage3_classified' AND score >= 4 THEN 2
      WHEN stage = 'stage3_classified' THEN 3
      WHEN stage LIKE 'stage%' THEN 4
      ELSE 0 END AS grp
  FROM m
), r AS (
  SELECT *, row_number() OVER (PARTITION BY grp ORDER BY created_at DESC) AS rn FROM g WHERE grp > 0
)
SELECT platform, keyword, stage, text, reason, score, is_lead FROM r
WHERE (grp = 1 AND rn <= 20) OR (grp = 2 AND rn <= 12) OR (grp = 3 AND rn <= 12) OR (grp = 4 AND rn <= 16)
ORDER BY grp, rn`, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewPostRow
	for rows.Next() {
		var r ReviewPostRow
		if err := rows.Scan(&r.Platform, &r.Keyword, &r.Stage, &r.Text, &r.Reason, &r.Score, &r.IsLead); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetKeywordReview returns the stored review, nil when there is none.
func (q *Queries) GetKeywordReview(ctx context.Context, workspaceID string) ([]byte, error) {
	var raw []byte
	err := q.db.QueryRow(ctx, `SELECT settings->'keyword_review' FROM workspaces WHERE id = $1`, workspaceID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}

// SaveKeywordReview replaces the stored review.
func (q *Queries) SaveKeywordReview(ctx context.Context, workspaceID string, review []byte) error {
	_, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY['keyword_review'], $2::jsonb, true)
WHERE id = $1`, workspaceID, string(review))
	return err
}

// SetKeywordReviewDecision records what the customer did with one
// suggestion ("added", "dismissed", "paused", ...), keyed by its id. It is a
// no-op when there is no stored review.
func (q *Queries) SetKeywordReviewDecision(ctx context.Context, workspaceID, suggestionID, decision string) (bool, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE workspaces
SET settings = jsonb_set(settings, ARRAY['keyword_review', 'decisions'],
                         COALESCE(settings->'keyword_review'->'decisions', '{}'::jsonb) || jsonb_build_object($2::text, $3::text), true)
WHERE id = $1 AND settings ? 'keyword_review'`, workspaceID, suggestionID, decision)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ListWorkspacesDueKeywordReview returns the Pro workspaces with active
// keywords whose last review (or onboarding) was before `before` and that
// have no scan waiting or running.
func (q *Queries) ListWorkspacesDueKeywordReview(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := q.db.Query(ctx, `
SELECT w.id::text FROM workspaces w
WHERE w.plan = 'pro' AND NOT w.scanning_paused
  AND EXISTS (SELECT 1 FROM keywords k WHERE k.workspace_id = w.id AND k.is_active)
  AND EXISTS (SELECT 1 FROM brand_profiles b WHERE b.workspace_id = w.id AND b.product <> '')
  AND COALESCE((w.settings->'keyword_review'->>'generated_at')::timestamptz, w.created_at) < $1
  AND COALESCE(w.settings->'manual_scan'->>'state', '') NOT IN ('requested', 'running')
ORDER BY w.id`, before)
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
