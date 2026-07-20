package database

import (
	"context"
	"fmt"
	"strings"
)

// ListMentionsComposedParams carries every inbox filter so they can be combined
// in a single query (the old per-filter sqlc queries were mutually exclusive —
// only the first one applied, silently dropping the rest).
type ListMentionsComposedParams struct {
	WorkspaceID string
	Tier        string // "", leads_ready, worth_watching, filtered
	Queue       string // "", auto_flowing, escalations
	Status      string
	Platform    string
	Intent      string
	Search      string
	Lim         int32
	Off         int32
}

// buildWhere returns the composed WHERE clause and its positional args. The tier
// predicates intentionally mirror CountMentionsByTier / the per-tier list
// queries so tier filtering stays consistent with the tier counts.
func (p ListMentionsComposedParams) buildWhere() (string, []any) {
	args := []any{p.WorkspaceID}
	clauses := []string{"workspace_id = $1"}

	add := func(clause string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	switch p.Tier {
	case "leads_ready":
		clauses = append(clauses, "relevance_score >= 7.0 AND intent IN ('buy_signal', 'recommendation_ask', 'complaint')")
	case "worth_watching":
		clauses = append(clauses, "relevance_score >= 4.0 AND relevance_score < 7.0")
	case "filtered":
		clauses = append(clauses,
			"NOT (COALESCE(relevance_score, 0) >= 7.0 AND COALESCE(intent IN ('buy_signal', 'recommendation_ask', 'complaint'), false))"+
				" AND NOT (COALESCE(relevance_score, 0) >= 4.0 AND COALESCE(relevance_score, 0) < 7.0)")
	}

	switch p.Queue {
	case "auto_flowing":
		clauses = append(clauses, leadIntentPredicate())
		clauses = append(clauses, "status NOT IN ('spam', 'archived')")
		clauses = append(clauses, `EXISTS (
			SELECT 1 FROM replies r
			WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
			AND r.status IN ('draft', 'approved')
		)`)
		clauses = append(clauses, "NOT COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)")
	case "escalations":
		clauses = append(clauses, "status NOT IN ('spam', 'archived')")
		clauses = append(clauses, `(
			COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)
			OR (
				`+leadIntentPredicate()+`
				AND status = 'new'
				AND NOT EXISTS (
					SELECT 1 FROM replies r
					WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
					AND r.status IN ('draft', 'approved', 'posted')
				)
			)
		)`)
	}
	if p.Status != "" {
		add("status = $%d", p.Status)
	}
	if p.Platform != "" {
		add("platform = $%d", p.Platform)
	}
	if p.Intent != "" {
		add("intent = $%d", p.Intent)
	}
	if p.Search != "" {
		add("content_tsv @@ plainto_tsquery('english', $%d)", p.Search)
	}
	return strings.Join(clauses, " AND "), args
}

func leadIntentPredicate() string {
	return "relevance_score >= 7.0 AND intent IN ('buy_signal', 'recommendation_ask', 'complaint')"
}

// CountMentionsByQueue returns counts for the action-oriented inbox queues.
func (q *Queries) CountMentionsByQueue(ctx context.Context, workspaceID string) ([]CountMentionsByQueueRow, error) {
	sql := `
SELECT 'auto_flowing' AS queue, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = $1
  AND ` + leadIntentPredicate() + `
  AND status NOT IN ('spam', 'archived')
  AND NOT COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)
  AND EXISTS (
    SELECT 1 FROM replies r
    WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
    AND r.status IN ('draft', 'approved')
  )
UNION ALL
SELECT 'escalations' AS queue, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = $1
  AND status NOT IN ('spam', 'archived')
  AND (
    COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)
    OR (
      ` + leadIntentPredicate() + `
      AND status = 'new'
      AND NOT EXISTS (
        SELECT 1 FROM replies r
        WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
        AND r.status IN ('draft', 'approved', 'posted')
      )
    )
  )`
	rows, err := q.db.Query(ctx, sql, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CountMentionsByQueueRow
	for rows.Next() {
		var i CountMentionsByQueueRow
		if err := rows.Scan(&i.Queue, &i.Count); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

type CountMentionsByQueueRow struct {
	Queue string `json:"queue"`
	Count int32  `json:"count"`
}

const mentionColumns = `id, workspace_id, keyword_id, platform, platform_id, url, title, content, content_tsv, author_username, author_profile_url, author_karma, author_account_age_days, relevance_score, intent, conversion_probability, status, assigned_to, platform_metadata, engagement_metrics, keyword_matches, platform_created_at, created_at, updated_at, content_embedding, scoring_metadata, awareness_level`

// ListMentionsComposed applies all provided filters together (ANDed), with
// pagination, ordered by recency.
func (q *Queries) ListMentionsComposed(ctx context.Context, p ListMentionsComposedParams) ([]Mention, error) {
	where, args := p.buildWhere()
	args = append(args, p.Lim, p.Off)
	sql := fmt.Sprintf(
		"SELECT %s FROM mentions WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		mentionColumns, where, len(args)-1, len(args),
	)
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Mention{}
	for rows.Next() {
		var i Mention
		if err := rows.Scan(
			&i.ID, &i.WorkspaceID, &i.KeywordID, &i.Platform, &i.PlatformID, &i.Url,
			&i.Title, &i.Content, &i.ContentTsv, &i.AuthorUsername, &i.AuthorProfileUrl,
			&i.AuthorKarma, &i.AuthorAccountAgeDays, &i.RelevanceScore, &i.Intent,
			&i.ConversionProbability, &i.Status, &i.AssignedTo, &i.PlatformMetadata,
			&i.EngagementMetrics, &i.KeywordMatches, &i.PlatformCreatedAt, &i.CreatedAt,
			&i.UpdatedAt, &i.ContentEmbedding, &i.ScoringMetadata, &i.AwarenessLevel,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// CountMentionsComposed returns the total number of mentions matching the same
// filters (so the API can report a real total, not just the current page size).
func (q *Queries) CountMentionsComposed(ctx context.Context, p ListMentionsComposedParams) (int64, error) {
	where, args := p.buildWhere()
	sql := "SELECT COUNT(*) FROM mentions WHERE " + where
	var total int64
	err := q.db.QueryRow(ctx, sql, args...).Scan(&total)
	return total, err
}
