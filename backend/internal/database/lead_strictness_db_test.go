package database

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed: the SQL lead rule moves the line for posts only. A confirmed
// company lead at 6.5 stays a lead under strict; an unconfirmed hiring
// signal at 5.0 does not become one under loose.
func TestLeadStrictnessSQLExemptsCompanySignalsDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('ls-'||gen_random_uuid(), 'ls', 'ls-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	add := func(pid string, score float64, meta string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, intent, relevance_score, platform_metadata, engagement_metrics, keyword_matches)
VALUES ($1, 'linkedin', $2, 'https://x/'||$2, 'a lead', 'new', 'buy_signal', $3, $4::jsonb, '{}', '{}')`, ws, pid, score, meta); err != nil {
			t.Fatalf("insert %s: %v", pid, err)
		}
	}
	const post, hiring = `{}`, `{"signal":"hiring"}`
	add("company-6.5", 6.5, hiring)
	add("unconfirmed-5.0", 5.0, hiring)
	add("post-7.0", 7.0, post)
	add("post-5.5", 5.5, post)

	q := New(pool)
	leads := func() []string {
		t.Helper()
		p := ListMentionsComposedParams{WorkspaceID: ws, Tier: "leads_ready", Lim: 50}
		rows, err := q.ListMentionsComposed(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		tiers, err := q.CountMentionsByTier(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.PlatformID
		}
		for _, c := range tiers {
			if c.Tier == "leads_ready" && int(c.Count) != len(ids) {
				t.Fatalf("tier count %d, composed list %v", c.Count, ids)
			}
		}
		slices.Sort(ids)
		return ids
	}
	for _, c := range []struct {
		level LeadStrictness
		want  []string
	}{
		{LeadStrict, []string{"company-6.5"}},
		{LeadBalanced, []string{"company-6.5", "post-7.0"}},
		{LeadLoose, []string{"company-6.5", "post-5.5", "post-7.0"}},
	} {
		if err := q.SetLeadStrictness(ctx, ws, c.level); err != nil {
			t.Fatal(err)
		}
		if got := leads(); !slices.Equal(got, c.want) {
			t.Errorf("%s: leads %v, want %v", c.level, got, c.want)
		}
	}
}
