package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Reply payoff: did a posted reply bring clicks and sign-ups? Clicks come from
// the /r/{code} redirect, sign-ups from the conversion API, both on the reply's
// utm_links row. Hand-written (no sqlc codegen).

// ErrReplyNotApproved: only an approved (or already posted) reply can be marked posted.
var ErrReplyNotApproved = errors.New("reply is not approved")

// MarkReplyPostedURL marks an approved (or already posted) reply posted,
// keeping the first posted_at, and records the URL of the posted comment when
// one is given ("" keeps the old). pgx.ErrNoRows when the workspace has no
// such reply, ErrReplyNotApproved when it is still a draft (or failed).
func (q *Queries) MarkReplyPostedURL(ctx context.Context, id, workspaceID, postedURL string) (Reply, error) {
	tag, err := q.db.Exec(ctx, `
UPDATE replies
SET status = 'posted', posted_at = COALESCE(posted_at, NOW()),
    posted_url = COALESCE(NULLIF($3, ''), posted_url), updated_at = NOW()
WHERE id = $1 AND workspace_id = $2 AND status IN ('approved', 'posted')`, id, workspaceID, postedURL)
	if err != nil {
		return Reply{}, err
	}
	r, err := q.GetReply(ctx, GetReplyParams{ID: id, WorkspaceID: workspaceID})
	if err == nil && tag.RowsAffected() == 0 {
		return Reply{}, ErrReplyNotApproved
	}
	return r, err
}

// ReplyPayoffRow is one reply that was posted (or whose link was clicked).
type ReplyPayoffRow struct {
	ReplyID   string     `json:"reply_id"`
	MentionID string     `json:"mention_id"`
	Platform  string     `json:"platform"`
	Title     string     `json:"title"`
	ThreadURL string     `json:"thread_url"`
	Keyword   string     `json:"keyword"`
	PostedAt  *time.Time `json:"posted_at"`
	PostedURL string     `json:"posted_url"`
	Clicks    int        `json:"clicks"`
	Signups   int        `json:"signups"`
}

// ListReplyPayoff returns the workspace's replies posted since `since`, plus
// approved ones whose tracked link was clicked (the customer posted it and
// never marked it), newest first.
// ponytail: capped at 1000 rows; page it if a workspace posts more per window.
func (q *Queries) ListReplyPayoff(ctx context.Context, workspaceID string, since time.Time) ([]ReplyPayoffRow, error) {
	rows, err := q.db.Query(ctx, `
SELECT r.id::text, m.id::text, m.platform::text, COALESCE(m.title, ''), m.url,
       COALESCE(k.term, m.keyword_matches[1], ''),
       r.posted_at, COALESCE(r.posted_url, ''),
       COALESCE(u.click_count, 0), COALESCE(u.signup_count, 0)
FROM replies r
JOIN mentions m ON m.id = r.mention_id AND m.workspace_id = r.workspace_id
LEFT JOIN keywords k ON k.id = m.keyword_id
LEFT JOIN utm_links u ON u.id = r.utm_link_id AND u.workspace_id = r.workspace_id
WHERE r.workspace_id = $1
  AND (r.status = 'posted' OR u.click_count > 0)
  AND COALESCE(r.posted_at, r.approved_at, r.created_at) >= $2
ORDER BY COALESCE(r.posted_at, r.approved_at, r.created_at) DESC
LIMIT 1000`, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReplyPayoffRow
	for rows.Next() {
		var p ReplyPayoffRow
		if err := rows.Scan(&p.ReplyID, &p.MentionID, &p.Platform, &p.Title, &p.ThreadURL, &p.Keyword,
			&p.PostedAt, &p.PostedURL, &p.Clicks, &p.Signups); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InTx runs fn on Queries bound to one transaction: committed when fn returns
// nil, rolled back otherwise. Works on a pool or inside a transaction (savepoint).
func (q *Queries) InTx(ctx context.Context, fn func(*Queries) error) error {
	b, ok := q.db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return errors.New("database: InTx needs a pool or transaction")
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit
	if err := fn(q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
