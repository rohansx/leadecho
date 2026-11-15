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
// fail, says why, leaves alone posts, old leads and leads the
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
	if _, _, _, err := m.rescreen(ctx, ws); err == nil {
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

	// A plan that does not apply names no roles: screening against it would
	// drop every posting, so the run fails and nothing is archived.
	setPlan := func(plan string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET settings = jsonb_set(settings, '{hiring_signals,plan}', $2::jsonb) WHERE id = $1`, ws, plan); err != nil {
			t.Fatal(err)
		}
	}
	setPlan(`{"applies":false,"roles":[]}`)
	if _, archived, _, err := m.rescreen(ctx, ws); err == nil || archived != 0 || planner.calls != 0 {
		t.Fatalf("without an applying plan: %d archived, %d screen calls, err %v; want an error and nothing done", archived, planner.calls, err)
	}
	var gone int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mentions WHERE workspace_id = $1 AND status = 'archived'`, ws).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("without an applying plan %d leads were archived (%v)", gone, err)
	}
	setPlan(`{"applies":true,"roles":["software engineer"],"max_company_size":5000}`)
	checked, archived, _, err := m.rescreen(ctx, ws)
	if err != nil || checked != 3 || archived != 2 {
		t.Fatalf("rescreen = %d checked, %d archived, %v; want 3 and 2", checked, archived, err)
	}
	// state returns the lead's status, why the re-screen archived it, and when.
	state := func(id string) (status, reason, at string) {
		t.Helper()
		var meta string
		if err := pool.QueryRow(ctx, `SELECT status::text, COALESCE(scoring_metadata->'rescreen'->>'reason', ''), COALESCE(scoring_metadata->'rescreen'->>'at', ''), scoring_metadata::text
FROM mentions WHERE id = $1 AND scoring_metadata->>'stage' = 'hiring_signal'`, id).Scan(&status, &reason, &at, &meta); err != nil {
			t.Fatal(err) // also fails if the scoring metadata already there was lost
		}
		// The customer's API returns scoring_metadata as stored: who asked is
		// in the ops audit log, not here.
		if strings.Contains(meta, `"by"`) {
			t.Fatalf("who asked is stored where the customer can read it: %s", meta)
		}
		return
	}
	_, _, archivedAt := state(agency)
	if s, why, _ := state(agency); s != "archived" || why != rescreenPostingReason || archivedAt == "" {
		t.Fatalf("agency = %s, %q, at %q", s, why, archivedAt)
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
	checked, archived, _, err = m.rescreen(ctx, ws)
	if err != nil || checked != 1 || archived != 0 {
		t.Fatalf("second run = %d checked, %d archived, %v; want 1 and 0", checked, archived, err)
	}
	if _, why, at := state(agency); why != rescreenPostingReason || at != archivedAt {
		t.Fatalf("a second run rewrote the record: %q at %q", why, at)
	}
	if planner.calls != 2 {
		t.Fatalf("the screen ran %d times, want once per run", planner.calls)
	}
}
