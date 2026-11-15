package database

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ListMentionsComposedParams carries every inbox filter so they can be combined
// in a single query (the old per-filter sqlc queries were mutually exclusive —
// only the first one applied, silently dropping the rest).
type ListMentionsComposedParams struct {
	WorkspaceID    string
	Tier           string // "", leads_ready, worth_watching, filtered
	Queue          string // "", action_required, auto_flowing, escalations, all
	ActionKind     string // "", ready_to_send, needs_draft, needs_review, older (action_required)
	EscalationKind string // legacy alias for action_kind on escalations queue
	Status         string
	Platform       string
	Intent         string
	Search         string
	Tag            string // scoring: one of the 13 intent tags
	Sentiment      string // scoring: positive|neutral|negative
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
		clauses = append(clauses, leadIntentPredicate())
	case "worth_watching":
		clauses = append(clauses, "relevance_score >= 4.0 AND NOT COALESCE("+leadIntentPredicate()+", false)")
	case "filtered":
		clauses = append(clauses,
			"NOT (COALESCE("+leadIntentPredicate()+", false))"+
				" AND COALESCE(relevance_score, 0) < 4.0")
	}

	switch p.Queue {
	case "action_required":
		clauses = append(clauses, actionRequiredPredicate())
		// Stale leads leave the queue but are not lost: "older" lists them.
		if p.ActionKind == "older" {
			clauses = append(clauses, "NOT "+freshPredicate())
		} else {
			clauses = append(clauses, freshPredicate())
		}
		switch p.ActionKind {
		case "ready_to_send":
			clauses = append(clauses, readyToSendPredicate())
		case "needs_draft":
			clauses = append(clauses, leadIntentPredicate())
			clauses = append(clauses, "status = 'new'")
			clauses = append(clauses, replyNoneExists())
		case "needs_review":
			clauses = append(clauses, escalationFlaggedPredicate())
		}
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
		// Hide spam/archived by default, but an explicit status filter for them
		// must win, or archived/spam mentions are unreachable from the UI.
		if p.Status != "spam" && p.Status != "archived" {
			clauses = append(clauses, "status NOT IN ('spam', 'archived')")
		}
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
		// The text of the post, and also its title and author: a company or a
		// person is what people look for ("toku"), and neither is in the body
		// of a hiring signal. ILIKE also finds names the English dictionary
		// search would stem away.
		add("(content_tsv @@ plainto_tsquery('english', $%[1]d) OR title ILIKE '%%' || $%[1]d || '%%' OR author_username ILIKE '%%' || $%[1]d || '%%')", p.Search)
	}
	// scoring: tag + sentiment filters
	if p.Tag != "" {
		add("$%d = ANY(intent_tags)", p.Tag)
	}
	if p.Sentiment != "" {
		add("sentiment = $%d", p.Sentiment)
	}
	return strings.Join(clauses, " AND "), args
}

// A lead is worth acting on while what it is about is recent: a post for
// PostLeadFreshDays, a company signal (job posting, prospect) for
// CompanyLeadFreshDays.
const (
	PostLeadFreshDays    = 7
	CompanyLeadFreshDays = 10
)

// freshPredicate is true while the post or posting behind a mention is recent
// enough to act on. Without a post time, when it was found counts.
func freshPredicate() string {
	return fmt.Sprintf(`(COALESCE(platform_created_at, created_at) >= now() - CASE
		WHEN COALESCE(platform_metadata->>'signal', '') <> '' THEN interval '%d days'
		ELSE interval '%d days' END)`, CompanyLeadFreshDays, PostLeadFreshDays)
}

// What makes a mention a lead: the one rule behind the inbox queue, the tier
// counts and the next-action badge.
const LeadMinScore = 6.0

var leadIntents = []IntentType{IntentTypeBuySignal, IntentTypeRecommendationAsk, IntentTypeComplaint, IntentTypeComparison}

// leadIntentPredicate is the rule as SQL.
func leadIntentPredicate() string {
	names := make([]string, len(leadIntents))
	for i, in := range leadIntents {
		names[i] = "'" + string(in) + "'"
	}
	return fmt.Sprintf("relevance_score >= %.1f AND intent IN (%s)", LeadMinScore, strings.Join(names, ", "))
}

// IsLead is the same rule for a row in hand.
func (m Mention) IsLead() bool {
	return m.RelevanceScore.Valid && m.RelevanceScore.Float32 >= LeadMinScore && m.Intent.Valid && slices.Contains(leadIntents, m.Intent.IntentType)
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

func readyToSendPredicate() string {
	return `(
		` + leadIntentPredicate() + `
		AND NOT COALESCE((scoring_metadata->>'needs_escalation')::boolean, false)
		AND ` + replyDraftOrApprovedExists() + `
	)`
}

// actionRequiredPredicate is what the inbox asks the customer to act on. A
// draft or a review flag does not keep a mention here once it is archived or
// marked spam: every list and count of the queue goes through this.
func actionRequiredPredicate() string {
	return `(status NOT IN ('spam', 'archived') AND (
		` + readyToSendPredicate() + `
		OR ` + escalationFlaggedPredicate() + `
		OR (
			` + leadIntentPredicate() + `
			AND status = 'new'
			AND ` + replyNoneExists() + `
		)
	))`
}

// CountMentionsByQueue returns counts for inbox queue tabs.
func (q *Queries) CountMentionsByQueue(ctx context.Context, workspaceID string) ([]CountMentionsByQueueRow, error) {
	sql := `
SELECT 'action_required' AS queue, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = $1
  AND ` + actionRequiredPredicate() + `
  AND ` + freshPredicate() + `
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

// CountActionSubcounts splits the action-required inbox into next-step
// slices, plus how many stale leads sit under "older".
func (q *Queries) CountActionSubcounts(ctx context.Context, workspaceID string) (ActionSubcounts, error) {
	sql := `
SELECT
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1 AND status NOT IN ('spam', 'archived')
     AND ` + freshPredicate() + `
     AND ` + readyToSendPredicate() + `
  ) AS ready_to_send,
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1 AND status NOT IN ('spam', 'archived')
     AND ` + freshPredicate() + `
     AND ` + leadIntentPredicate() + `
     AND status = 'new'
     AND ` + replyNoneExists() + `
  ) AS needs_draft,
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1 AND status NOT IN ('spam', 'archived')
     AND ` + freshPredicate() + `
     AND ` + escalationFlaggedPredicate() + `
  ) AS needs_review,
  (SELECT COUNT(*)::int FROM mentions
   WHERE workspace_id = $1
     AND ` + actionRequiredPredicate() + `
     AND NOT ` + freshPredicate() + `
  ) AS older`
	var s ActionSubcounts
	err := q.db.QueryRow(ctx, sql, workspaceID).Scan(&s.ReadyToSend, &s.NeedsDraft, &s.NeedsReview, &s.Older)
	return s, err
}

type ActionSubcounts struct {
	ReadyToSend int32 `json:"ready_to_send"`
	NeedsDraft  int32 `json:"needs_draft"`
	NeedsReview int32 `json:"needs_review"`
	Older       int32 `json:"older"`
}

// CountEscalationSubcounts is kept for backward-compatible API responses.
func (q *Queries) CountEscalationSubcounts(ctx context.Context, workspaceID string) (EscalationSubcounts, error) {
	s, err := q.CountActionSubcounts(ctx, workspaceID)
	if err != nil {
		return EscalationSubcounts{}, err
	}
	return EscalationSubcounts{NeedsDraft: s.NeedsDraft, Flagged: s.NeedsReview}, nil
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

// MentionIDsWithDraftOrApprovedReply returns mention IDs that have a draft/approved reply.
func (q *Queries) MentionIDsWithDraftOrApprovedReply(ctx context.Context, workspaceID string, mentionIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(mentionIDs))
	if len(mentionIDs) == 0 {
		return out, nil
	}
	rows, err := q.db.Query(ctx, `
SELECT DISTINCT mention_id::text
FROM replies
WHERE workspace_id = $1
  AND mention_id = ANY($2::uuid[])
  AND status IN ('draft', 'approved')`, workspaceID, mentionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

const mentionColumns = `id, workspace_id, keyword_id, platform, platform_id, url, title, content, content_tsv, author_username, author_profile_url, author_karma, author_account_age_days, relevance_score, intent, conversion_probability, status, assigned_to, platform_metadata, engagement_metrics, keyword_matches, platform_created_at, created_at, updated_at, content_embedding, scoring_metadata, awareness_level, intent_tags, sentiment, canonical_url, content_simhash`

// ListMentionsComposed applies all provided filters together (ANDed), with
// pagination. The action queue and browse-all list the best score first, then
// the most recent post: having a draft does not move a lead up. Many leads
// share a score and a post time, so the id settles ties: without it a lead
// can repeat on, or be missing from, the next page.
func (q *Queries) ListMentionsComposed(ctx context.Context, p ListMentionsComposedParams) ([]Mention, error) {
	where, args := p.buildWhere()
	orderBy := "created_at DESC"
	switch p.Queue {
	case "action_required":
		orderBy = "COALESCE(relevance_score, 0) DESC, COALESCE(platform_created_at, created_at) DESC, id"
	case "all":
		if p.Search == "" {
			orderBy = "COALESCE(relevance_score, 0) DESC, created_at DESC"
		}
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
			&i.IntentTags, &i.Sentiment, &i.CanonicalUrl, &i.ContentSimhash,
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
