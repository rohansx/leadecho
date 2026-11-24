package monitor

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/billing"
	"leadecho/internal/database"
)

// DB-backed: a Solo workspace has paid data (for Reddit), yet hiring signals
// and prospecting stay off because LinkedIn is locked; Starter runs them.
func TestHiringOffForSoloDB(t *testing.T) {
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
	slug := fmt.Sprintf("solo-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: slug, Name: "Solo locks", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID) //nolint:errcheck

	m := &Monitor{q: q, credits: billing.New(q, true), logger: zerolog.Nop(), hiringPlanner: fakeHiringPlanner{}}
	setPlan := func(plan string) {
		p, _ := billing.PlanByID(plan)
		if err := q.OpsSetPlan(ctx, database.OpsSetPlanParams{WorkspaceID: ws.ID, Plan: p.ID, PlanSource: "comp", KeywordLimit: int32(p.KeywordLimit)}); err != nil {
			t.Fatal(err)
		}
		if err := q.OpsSetPaidData(ctx, database.OpsSetPaidDataParams{WorkspaceID: ws.ID, PaidData: p.PaidData}); err != nil {
			t.Fatal(err)
		}
	}

	setPlan("solo")
	locked := m.credits.LockedPlatforms(ctx, ws.ID)
	if !locked["linkedin"] || !locked["twitter"] || locked["reddit"] {
		t.Fatalf("solo locked = %v, want X and LinkedIn only", locked)
	}
	if !m.linkedInLocked(ctx, ws.ID) {
		t.Fatal("hiring and prospecting must stay off for Solo")
	}

	setPlan("starter")
	if m.linkedInLocked(ctx, ws.ID) || len(m.credits.LockedPlatforms(ctx, ws.ID)) != 0 {
		t.Fatal("starter has every platform")
	}
}
