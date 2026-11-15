package monitor

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: the model fails to write the plan on a new workspace's first
// company scan. Nothing was paid for, so the day's claim is not kept: the
// next run is due within the hour, and the checklist says so.
func TestScanHiringPlanFailureRetriesWithinTheHourDB(t *testing.T) {
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
	q := database.New(pool)
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('hp-'||gen_random_uuid(), 'hp', 'hp-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, product, overview, target_audience) VALUES ($1, 'Assessments', 'Technical skill assessments', 'Engineering leaders')`, ws); err != nil {
		t.Fatal(err)
	}
	client, calls := fakeTreg(t, func(string, map[string]any) (int, string) { return 400, `{"error":"no paid call is expected"}` })
	// fakeHiringPlanner.PlanHiringSignals always fails.
	m := &Monitor{q: q, logger: zerolog.Nop(), treg: client, hiringPlanner: fakeHiringPlanner{}}

	m.scanHiring(ctx, ws)

	if len(*calls) != 0 {
		t.Fatalf("paid calls after a failed plan: %+v", *calls)
	}
	raw, err := q.GetWorkspaceSettingsKey(ctx, ws, hiringSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state, next := CompanyScanProgress(raw, now)
	if state != CompanyScanRetry || next.After(now.Add(hiringRetry+time.Minute)) || next.Before(now.Add(hiringRetry-5*time.Minute)) {
		t.Fatalf("after a failed plan: %s, next %v (now %v), state %s", state, next, now, raw)
	}
	// Not at once either: the next tick does not call the model again.
	if claimed, err := q.ClaimSettingsRun(ctx, ws, hiringSettingsKey, hiringInterval); err != nil || claimed {
		t.Fatalf("claimable again at once: %v %v", claimed, err)
	}
}
