package database

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed: the default inbox queue lists the best lead first whether or not
// it has a draft, leaves out what the customer archived and what has gone
// stale (still reachable under "older"), and its counts say the same.
func TestActionRequiredQueueDB(t *testing.T) {
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
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('aq-'||gen_random_uuid(), 'aq', 'aq-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	// add stores a buy-signal lead posted `posted` days ago (-1: no post
	// time) and found `found` days ago.
	add := func(pid, status string, score float64, posted, found int, platformMeta string, draft bool) {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, intent, relevance_score, platform_metadata,
                      platform_created_at, created_at, engagement_metrics, keyword_matches)
VALUES ($1, 'linkedin', $2, 'https://x/'||$2, 'a lead', $3::mention_status, 'buy_signal', $4, $5::jsonb,
        CASE WHEN $6 < 0 THEN NULL ELSE now() - make_interval(days => $6, mins => 5) END,
        now() - make_interval(days => $7, mins => 5), '{}', '{}')
RETURNING id::text`, ws, pid, status, score, platformMeta, posted, found).Scan(&id); err != nil {
			t.Fatalf("insert %s: %v", pid, err)
		}
		if draft {
			if _, err := pool.Exec(ctx, `INSERT INTO replies (mention_id, workspace_id, content) VALUES ($1, $2, 'hi')`, id, ws); err != nil {
				t.Fatalf("draft for %s: %v", pid, err)
			}
		}
	}
	const post, hiring = `{}`, `{"signal":"hiring"}`
	add("drafted-low", "new", 6.5, 1, 1, hiring, true)
	add("best", "new", 8.5, 2, 2, post, false)
	add("same-score-newer", "new", 6.5, 0, 0, post, false)
	add("archived-drafted", "archived", 7.5, 1, 1, post, true) // dismissed by the customer
	add("old-post", "new", 9.0, 8, 1, post, true)              // posts go stale after 7 days
	add("fresh-posting", "new", 7.5, 9, 9, hiring, true)       // company signals after 10
	add("old-posting", "new", 7.5, 11, 1, hiring, true)        //
	add("found-long-ago", "new", 7.0, -1, 8, post, false)      // no post time: the found time counts
	add("found-recently", "new", 7.0, -1, 3, post, false)      //
	add("spam-flagged", "spam", 9.5, 0, 0, `{}`, false)        //
	add("old-archived", "archived", 9.0, 20, 20, post, true)   // archived stays out of "older" too

	q := New(pool)
	list := func(kind string) []string {
		t.Helper()
		p := ListMentionsComposedParams{WorkspaceID: ws, Queue: "action_required", ActionKind: kind, Lim: 50}
		rows, err := q.ListMentionsComposed(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		total, err := q.CountMentionsComposed(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(rows) {
			t.Fatalf("%q: total %d, listed %d", kind, total, len(rows))
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.PlatformID
		}
		return ids
	}
	same := func(name string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", name, got, want)
			}
		}
	}

	queue := list("")
	same("default queue", queue, []string{"best", "fresh-posting", "found-recently", "same-score-newer", "drafted-low"})
	older := list("older")
	same("older", older, []string{"old-post", "old-posting", "found-long-ago"})
	ready, needsDraft := list("ready_to_send"), list("needs_draft")
	same("ready to send", ready, []string{"fresh-posting", "drafted-low"})
	same("needs draft", needsDraft, []string{"best", "found-recently", "same-score-newer"})

	counts, err := q.CountMentionsByQueue(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range counts {
		if c.Queue == "action_required" && int(c.Count) != len(queue) {
			t.Fatalf("queue count = %d, the queue lists %d", c.Count, len(queue))
		}
	}
	sub, err := q.CountActionSubcounts(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if int(sub.ReadyToSend) != len(ready) || int(sub.NeedsDraft) != len(needsDraft) || sub.NeedsReview != 0 || int(sub.Older) != len(older) {
		t.Fatalf("subcounts = %+v, want %d ready, %d needing a draft, %d older", sub, len(ready), len(needsDraft), len(older))
	}
}
