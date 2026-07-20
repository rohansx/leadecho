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
	WorkspaceID    string
	Tier           string // "", leads_ready, worth_watching, filtered
	Queue          string // "", auto_flowing, escalations, all
	EscalationKind string // "", needs_draft, flagged (only when Queue=escalations)
	Status         string
	Platform       string
	Intent         string
	Search         string
	Lim            int32
	Off            int32
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
		clauses = append(clauses, replyDraftOrApprovedExists())
		clauses = append(clauses, "NOT COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)")
	case "escalations":
		clauses = append(clauses, "status NOT IN ('spam', 'archived')")
		switch p.EscalationKind {
		case "needs_draft":
			clauses = append(clauses, leadIntentPredicate())
			clauses = append(clauses, "status = 'new'")
			clauses = append(clauses, replyNoneExists())
		case "flagged":
			clauses = append(clauses, escalationFlaggedPredicate())
		default:
			clauses = append(clauses, `(
				`+escalationFlaggedPredicate()+`
				OR (
					`+leadIntentPredicate()+`
					AND status = 'new'
					AND `+replyNoneExists()+`
				)
			)`)
		}
	case "all":
		clauses = append(clauses, "status NOT IN ('spam', 'archived')")
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

func escalationFlaggedPredicate() string {
	return "COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)"
}

func replyDraftOrApprovedExists() string {
	return `EXISTS (
			SELECT 1 FROM replies r
			WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
			AND r.status IN ('draft', 'approved')
		)`
}

func replyNoneExists() string {
	return `NOT EXISTS (
					SELECT 1 FROM replies r
					WHERE r.mention_id = mentions.id AND r.workspace_id = mentions.workspace_id
					AND r.status IN ('draft', 'approved', 'posted')
				)`
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
  AND ` + replyDraftOrApprovedExists() + `
UNION ALL
SELECT 'escalations' AS queue, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = $1
  AND status NOT IN ('spam', 'archived')
  AND (
    ` + escalationFlaggedPredicate() + `
    OR (
      ` + leadIntentPredicate() + `
      AND status = 'new'
      AND ` + replyNoneExists() + `
    )
  )
UNION ALL
SELECT 'all' AS queue, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = $1
  AND status NOT IN ('spam', 'archived')`
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

// CountEscalationSubcounts splits the escalations queue into actionable slices.
func (q *Queries) CountEscalationSubcounts(ctx context.Context, workspaceID string) (EscalationSubcounts, error) {
	sql := `
SELECT
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1 AND status NOT IN ('spam', 'archived')
     AND ` + leadIntentPredicate() + `
     AND status = 'new'
     AND ` + replyNoneExists() + `
  ) AS needs_draft,
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1 AND status NOT IN ('spam', 'archived')
     AND ` + escalationFlaggedPredicate() + `
  ) AS flagged`
	var s EscalationSubcounts
	err := q.db.QueryRow(ctx, sql, workspaceID).Scan(&s.NeedsDraft, &s.Flagged)
	return s, err
}

type EscalationSubcounts struct {
	NeedsDraft int32 `json:"needs_draft"`
	Flagged    int32 `json:"flagged"`
}

type CountMentionsByQueueRow struct {
	Queue string `json:"queue"`
	Count int32  `json:"count"`
}

// CountMentionsByPlatformForQueue returns per-platform totals for the same
// filter set as ListMentionsComposed (so inbox platform pills match the list).
func (q *Queries) CountMentionsByPlatformForQueue(ctx context.Context, p ListMentionsComposedParams) ([]CountMentionsByPlatformRow, error) {
	where, args := p.buildWhere()
	sql := fmt.Sprintf(
		"SELECT platform, COUNT(*)::int AS count FROM mentions WHERE %s GROUP BY platform ORDER BY count DESC",
		where,
	)
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CountMentionsByPlatformRow
	for rows.Next() {
		var i CountMentionsByPlatformRow
		if err := rows.Scan(&i.Platform, &i.Count); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const mentionColumns = `id, workspace_id, keyword_id, platform, platform_id, url, title, content, content_tsv, author_username, author_profile_url, author_karma, author_account_age_days, relevance_score, intent, conversion_probability, status, assigned_to, platform_metadata, engagement_metrics, keyword_matches, platform_created_at, created_at, updated_at, content_embedding, scoring_metadata, awareness_level`

// ListMentionsComposed applies all provided filters together (ANDed), with
// pagination, ordered by recency (action queues) or score (browse-all).
func (q *Queries) ListMentionsComposed(ctx context.Context, p ListMentionsComposedParams) ([]Mention, error) {
	where, args := p.buildWhere()
	orderBy := "created_at DESC"
	if p.Queue == "all" && p.Search == "" {
		orderBy = "COALESCE(relevance_score, 0) DESC, created_at DESC"
	}
	args = append(args, p.Lim, p.Off)
	sql := fmt.Sprintf(
		"SELECT %s FROM mentions WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d",
		mentionColumns, where, orderBy, len(args)-1, len(args),
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
