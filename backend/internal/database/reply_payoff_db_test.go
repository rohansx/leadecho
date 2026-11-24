package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed: marking posted keeps the first posted_at and the pasted URL;
// the payoff list carries the link's clicks/sign-ups, is workspace-scoped and
// leaves out unposted, unclicked drafts; a draft cannot be marked posted.
func TestReplyPayoffDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := New(pool)

	newWS := func() string {
		var ws string
		if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('rp-'||gen_random_uuid(), 'rp', 'rp-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws) })
		return ws
	}
	newReply := func(ws, pid string) Reply {
		var mid string
		if err := pool.QueryRow(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, title, content, keyword_matches)
VALUES ($1, 'reddit', $2, 'https://reddit.com/r/x/'||$2, 'need a crm', 'post', '{crm}') RETURNING id`, ws, pid).Scan(&mid); err != nil {
			t.Fatal(err)
		}
		r, err := q.CreateReply(ctx, CreateReplyParams{MentionID: mid, WorkspaceID: ws, Content: "hi", Status: ReplyStatusDraft})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ws, other := newWS(), newWS()
	posted := newReply(ws, "p1")
	_ = newReply(ws, "p2") // a draft, never posted or clicked: not listed

	link, err := q.CreateUTMLink(ctx, CreateUTMLinkParams{WorkspaceID: ws, Code: "rp" + posted.ID[:8], DestinationUrl: "https://x.co",
		UtmSource: "leadecho", UtmMedium: "social_reply", UtmContent: pgtype.Text{String: posted.ID, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE replies SET utm_link_id = $1 WHERE id = $2`, link.ID, posted.ID); err != nil {
		t.Fatal(err)
	}
	_ = q.IncrementUTMClicks(ctx, link.Code)
	_ = q.IncrementUTMClicks(ctx, link.Code)
	if _, err := q.RecordUTMConversion(ctx, RecordUTMConversionParams{Code: link.Code}); err != nil {
		t.Fatal(err)
	}

	if _, err := q.MarkReplyPostedURL(ctx, posted.ID, ws, ""); !errors.Is(err, ErrReplyNotApproved) {
		t.Fatalf("a draft must not be marked posted: %v", err)
	}
	if _, err := q.MarkReplyPostedURL(ctx, "00000000-0000-0000-0000-000000000000", ws, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown reply: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE replies SET status = 'approved' WHERE id = $1`, posted.ID); err != nil {
		t.Fatal(err)
	}
	first, err := q.MarkReplyPostedURL(ctx, posted.ID, ws, "https://reddit.com/r/x/comments/abc")
	if err != nil || first.Status != ReplyStatusPosted || !first.PostedAt.Valid {
		t.Fatalf("mark posted: %+v %v", first, err)
	}
	again, err := q.MarkReplyPostedURL(ctx, posted.ID, ws, "")
	if err != nil || !again.PostedAt.Time.Equal(first.PostedAt.Time) {
		t.Fatalf("re-mark must keep posted_at: %v", err)
	}
	if _, err := q.MarkReplyPostedURL(ctx, posted.ID, other, "https://evil.example"); err == nil {
		t.Fatal("another workspace marked the reply posted")
	}

	rows, err := q.ListReplyPayoff(ctx, ws, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want only the posted reply, got %+v", rows)
	}
	got := rows[0]
	if got.ReplyID != posted.ID || got.Clicks != 2 || got.Signups != 1 || got.Keyword != "crm" ||
		got.PostedURL != "https://reddit.com/r/x/comments/abc" || got.PostedAt == nil || got.Title != "need a crm" {
		t.Fatalf("row = %+v", got)
	}
	if rows, _ := q.ListReplyPayoff(ctx, other, time.Now().Add(-time.Hour)); len(rows) != 0 {
		t.Fatalf("other workspace sees %+v", rows)
	}
}
