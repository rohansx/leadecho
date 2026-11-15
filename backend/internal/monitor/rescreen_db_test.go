package monitor

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// screenOnly is a planner that can only screen: a re-screen that planned,
// researched or drafted anything would panic on the nil interface.
type screenOnly struct {
	HiringPlanner
	calls int
	drop  string // the company the screen drops
}

func (s *screenOnly) ScreenHiringJobs(_ context.Context, _ string, in ai.HiringScreen) ([]ai.ProspectFit, error) {
	s.calls++
	var fits []ai.ProspectFit
	for i, j := range in.Jobs {
		if j.Company != s.drop {
			fits = append(fits, ai.ProspectFit{Index: i})
		}
	}
	return fits, nil
}

// DB-backed: a re-screen archives the workspace's recent company leads that
// fail, says why and who asked, leaves alone posts, old leads and leads the
// customer acted on, and changes nothing when run again. The monitor has no
// data provider at all: nothing paid can be called.
func TestRescreenCompanyLeadsDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings) VALUES ('rsc-'||gen_random_uuid(), 'rsc', 'rsc-'||gen_random_uuid(),
		'{"hiring_signals":{"plan":{"applies":true,"roles":["software engineer"],"max_company_size":5000}}}') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	q := database.New(pool)
	m := &Monitor{q: q, logger: zerolog.Nop()}

	// Without a profile there is nothing to screen against.
	if _, _, err := m.rescreen(ctx, ws, "ops@example.com"); err == nil {
		t.Fatal("a workspace without a profile must not be re-screened")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, product, overview, target_audience) VALUES ($1, 'Skill tests', 'Assessments for hiring engineers', 'Engineering leaders')`, ws); err != nil {
		t.Fatal(err)
	}

	add := func(pid, status, platformMeta string, foundDaysAgo int) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, platform_metadata, scoring_metadata, created_at, engagement_metrics, keyword_matches)
VALUES ($1, 'linkedin', $2, 'https://x/'||$2, 'a lead', $3::mention_status, $4::jsonb, '{"stage":"hiring_signal"}', now() - make_interval(days => $5), '{}', '{}')
RETURNING id::text`, ws, pid, status, platformMeta, foundDaysAgo).Scan(&id); err != nil {
			t.Fatalf("insert %s: %v", pid, err)
		}
		return id
	}
	hiring := func(company string) string {
		return `{"signal":"hiring","company":"` + company + `","job_title":"Software Engineer"}`
	}
	buyer := add("buyer", "new", hiring("Acme"), 2)
	agency := add("agency", "new", hiring("Staffing Co"), 9)
	ruledOut := add("ruled-out", "new", `{"signal":"prospect","company":"Rival","research":{"not_a_buyer":"It sells a product like this one."}}`, 3)
	replied := add("replied", "replied", hiring("Staffing Co"), 3)
	tooOld := add("too-old", "new", hiring("Staffing Co"), 31)
	post := add("post", "new", `{}`, 1)

	planner := &screenOnly{drop: "Staffing Co"}
	m.hiringPlanner = planner
	checked, archived, err := m.rescreen(ctx, ws, "ops@example.com")
	if err != nil || checked != 3 || archived != 2 {
		t.Fatalf("rescreen = %d checked, %d archived, %v; want 3 and 2", checked, archived, err)
	}
	state := func(id string) (status, reason, by string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT status::text, COALESCE(scoring_metadata->'rescreen'->>'reason', ''), COALESCE(scoring_metadata->'rescreen'->>'by', '')
FROM mentions WHERE id = $1 AND scoring_metadata->>'stage' = 'hiring_signal'`, id).Scan(&status, &reason, &by); err != nil {
			t.Fatal(err) // also fails if the scoring metadata already there was lost
		}
		return
	}
	if s, why, by := state(agency); s != "archived" || why != rescreenPostingReason || by != "ops@example.com" {
		t.Fatalf("agency = %s, %q, by %q", s, why, by)
	}
	if s, why, _ := state(ruledOut); s != "archived" || !strings.Contains(why, "not a buyer") {
		t.Fatalf("ruled out = %s, %q", s, why)
	}
	for name, tc := range map[string]struct{ id, want string }{"buyer": {buyer, "new"}, "replied": {replied, "replied"}, "too old": {tooOld, "new"}, "post": {post, "new"}} {
		if s, why, _ := state(tc.id); s != tc.want || why != "" {
			t.Errorf("%s = %s, %q; want it left as %s", name, s, why, tc.want)
		}
	}

	// Again: only the lead that passed is still there to check.
	checked, archived, err = m.rescreen(ctx, ws, "someone-else@example.com")
	if err != nil || checked != 1 || archived != 0 {
		t.Fatalf("second run = %d checked, %d archived, %v; want 1 and 0", checked, archived, err)
	}
	if _, _, by := state(agency); by != "ops@example.com" {
		t.Fatalf("a second run rewrote the record: by %q", by)
	}
	if planner.calls != 2 {
		t.Fatalf("the screen ran %d times, want once per run", planner.calls)
	}
}
