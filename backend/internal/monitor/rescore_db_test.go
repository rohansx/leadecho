package monitor

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/database"
)

// DB-backed: a re-score picks up posts scored under older rules and leaves
// alone what the customer acted on, what is already a lead, company signals
// and posts nobody scored yet. Demand takes a non-buyer's ask on its own,
// and a buyer's only above the relevance floor.
func TestRescoreSelectionAndDemandAsksDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('rs-'||gen_random_uuid(), 'rs', 'rs-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	add := func(pid, status, scoring, platformMeta string, score *float64) string {
		var id string
		if err := pool.QueryRow(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, scoring_metadata, platform_metadata, relevance_score, engagement_metrics, keyword_matches)
VALUES ($1, 'reddit', $2, 'https://x/'||$2, 'a post long enough to be scored by the rules', $3::mention_status, COALESCE(NULLIF($4,''),'{}')::jsonb, $5::jsonb, $6, '{}', '{}')
RETURNING id::text`, ws, pid, status, scoring, platformMeta, score).Scan(&id); err != nil {
			t.Fatalf("insert %s: %v", pid, err)
		}
		return id
	}
	f := func(v float64) *float64 { return &v }
	old := add("old", "new", `{"stage":"stage3_classified","author_role":"buyer","ask":"A way to screen"}`, `{}`, f(5))
	filtered := add("filtered", "new", `{"stage":"stage1_spam_filter"}`, `{}`, nil)
	add("current", "new", `{"stage":"stage3_classified","rubric":2}`, `{}`, f(3))
	add("acted", "archived", `{"stage":"stage3_classified"}`, `{}`, f(3))
	add("hiring", "new", `{"stage":"stage3_classified"}`, `{"signal":"hiring"}`, f(7))
	add("unscored", "new", ``, `{}`, nil)
	add("tooshort", "new", `{"stage":"stage0_rules"}`, `{}`, nil)
	lead := add("lead", "new", `{"stage":"stage3_classified"}`, `{}`, f(7))
	if _, err := pool.Exec(ctx, `INSERT INTO leads (workspace_id, mention_id, stage) VALUES ($1, $2, 'prospect')`, ws, lead); err != nil {
		t.Fatal(err)
	}
	candidate := add("candidate", "new", `{"stage":"stage3_classified","rubric":2,"author_role":"non_buyer","ask":"To be told how it is graded"}`, `{}`, f(2))
	add("lowbuyer", "new", `{"stage":"stage3_classified","rubric":2,"author_role":"buyer","ask":"Off-topic need"}`, `{}`, f(2))

	q := database.New(pool)
	got, err := q.ListMentionsToRescore(ctx, ws, time.Now().Add(-time.Hour), ScoringRubric, 50)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if len(got) != 2 || !ids[old] || !ids[filtered] {
		t.Fatalf("re-score picked %d posts %v, want the classified and the filtered post scored under older rules", len(got), ids)
	}

	asks, err := q.ListMentionAsks(ctx, ws, time.Now().Add(-time.Hour), 4.0, 50)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, a := range asks {
		roles[a.MentionID] = a.AuthorRole
	}
	if len(asks) != 2 || roles[old] != "buyer" || roles[candidate] != "non_buyer" {
		t.Fatalf("demand asks = %v, want the buyer above the floor and the non-buyer", roles)
	}
}
