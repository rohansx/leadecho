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

// AskSignal is what the workspace knows about the post behind an ask.
type AskSignal struct {
	MentionID string
	Intent    string
	Score     float64
	Stage     string // the lead's stage; "" when the post is not a lead
	Content   string // the post, clipped
}

// ListAskSignals returns intent, score, lead stage and the start of the post
// for the given mentions of the workspace. Hand-written.
func (q *Queries) ListAskSignals(ctx context.Context, workspaceID string, mentionIDs []string) ([]AskSignal, error) {
	rows, err := q.db.Query(ctx, `
SELECT m.id::text, COALESCE(m.intent::text, ''), COALESCE(m.relevance_score, 0)::float8,
       COALESCE((SELECT l.stage::text FROM leads l WHERE l.workspace_id = m.workspace_id AND l.mention_id = m.id
                 ORDER BY l.updated_at DESC LIMIT 1), ''),
       LEFT(COALESCE(m.title, '') || ' ' || m.content, 2000)
FROM mentions m
WHERE m.workspace_id = $1 AND m.id = ANY($2::uuid[])`, workspaceID, mentionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AskSignal
	for rows.Next() {
		var s AskSignal
		if err := rows.Scan(&s.MentionID, &s.Intent, &s.Score, &s.Stage, &s.Content); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListMentionAsks returns the asks from posts scored since `since`, newest
// first. Left out: spam and the workspace's company signals (a company, not
// a person stating a need). A buyer's or an unknown author's post needs a
// relevance of at least minRelevance. A non-buyer's post (a job seeker, say)
// is scored low by rule however much it says about the market, so it is
// taken on its ask alone: the scorer only writes one for a non-buyer when
// the need is about the problem the product solves. The same goes for a
// proof point (a buyer showing how they already solved the problem).
// Hand-written (no sqlc codegen needed).
func (q *Queries) ListMentionAsks(ctx context.Context, workspaceID string, since time.Time, minRelevance float64, limit int32) ([]MentionAsk, error) {
	rows, err := q.db.Query(ctx, `
SELECT id::text, scoring_metadata->>'ask', COALESCE(scoring_metadata->>'author_role', ''),
       platform::text, url, COALESCE(author_username, ''), COALESCE(platform_created_at, created_at)
FROM mentions
WHERE workspace_id = $1 AND created_at >= $2
  AND COALESCE(scoring_metadata->>'ask', '') <> ''
  AND status <> 'spam'
  AND (COALESCE(relevance_score, 0) >= $4 OR COALESCE(scoring_metadata->>'author_role', '') = 'non_buyer'
       OR COALESCE(scoring_metadata->>'post_type', '') = 'proof_point')
  AND COALESCE(platform_metadata->>'signal', '') = ''
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
