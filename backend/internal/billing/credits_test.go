package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/database"
)

func TestPlans(t *testing.T) {
	cases := []struct {
		id             string
		price, credits int
		keywordLimit   int
	}{
		{"free", 0, 300, 3},
		{"starter", 4900, 5000, 15},
		{"pro", 14900, 20000, 50},
	}
	for _, c := range cases {
		p, ok := PlanByID(c.id)
		if !ok || p.PriceCents != c.price || p.MonthlyCredits != c.credits || p.KeywordLimit != c.keywordLimit {
			t.Errorf("plan %s = %+v ok=%v", c.id, p, ok)
		}
	}
	if _, ok := PlanByID("enterprise"); ok {
		t.Error("unknown plan must not resolve")
	}
}

// A nil or unenforced Credits (self-host, dev, billing not configured) never blocks.
func TestCreditsNotEnforced(t *testing.T) {
	ctx := context.Background()
	for _, c := range []*Credits{nil, New(nil, false)} {
		if got, err := c.Take(ctx, "ws", 7, ReasonPost); err != nil || got != 7 {
			t.Fatalf("Take = %d %v, want 7 nil", got, err)
		}
		if err := c.Spend(ctx, "ws", CostDraft, ReasonDraft); err != nil {
			t.Fatalf("Spend = %v", err)
		}
		if c.Enforced() {
			t.Fatal("Enforced() must be false")
		}
	}
}

func newWorkspace(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q *database.Queries, balance int) string {
	t.Helper()
	n := time.Now().UnixNano()
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprintf("org_bill_%d", n), Name: "Billing", Slug: fmt.Sprintf("bill-%d", n)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET credit_balance = $2 WHERE id = $1`, ws.ID, balance); err != nil {
		t.Fatal(err)
	}
	return ws.ID
}

func balance(t *testing.T, ctx context.Context, q *database.Queries, ws string) int {
	t.Helper()
	st, err := q.GetBillingState(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	return int(st.CreditBalance)
}

// DB-backed: runs when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestCreditsDB(t *testing.T) {
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
	c := New(q, true)

	t.Run("take is partial and never overdraws", func(t *testing.T) {
		ws := newWorkspace(t, ctx, pool, q, 5)
		if got, err := c.Take(ctx, ws, 3, ReasonPost); err != nil || got != 3 {
			t.Fatalf("first take = %d %v", got, err)
		}
		if got, _ := c.Take(ctx, ws, 10, ReasonPost); got != 2 {
			t.Fatalf("second take = %d, want the remaining 2", got)
		}
		if got, _ := c.Take(ctx, ws, 4, ReasonPost); got != 0 {
			t.Fatalf("empty take = %d, want 0", got)
		}
		if b := balance(t, ctx, q, ws); b != 0 {
			t.Fatalf("balance = %d, want 0", b)
		}
		usage, err := q.CreditUsageSince(ctx, database.CreditUsageSinceParams{WorkspaceID: ws, Since: pgDate(time.Now().AddDate(0, 0, -1))})
		if err != nil || len(usage) != 1 || usage[0].Reason != ReasonPost || usage[0].Credits != 5 {
			t.Fatalf("usage = %+v %v, want post=5", usage, err)
		}
	})

	t.Run("balance and refund", func(t *testing.T) {
		ws := newWorkspace(t, ctx, pool, q, 4)
		if b, err := c.Balance(ctx, ws); err != nil || b != 4 {
			t.Fatalf("balance = %d %v", b, err)
		}
		_, _ = c.Take(ctx, ws, 4, ReasonSearch)
		c.Refund(ctx, ws, 1, ReasonSearch)
		if b, _ := c.Balance(ctx, ws); b != 1 {
			t.Fatalf("after refund = %d, want 1", b)
		}
		if b, err := New(nil, false).Balance(ctx, ws); err != nil || b <= 0 {
			t.Fatalf("unenforced balance = %d %v, want a positive sentinel", b, err)
		}
	})

	t.Run("spend is all or nothing", func(t *testing.T) {
		ws := newWorkspace(t, ctx, pool, q, 7)
		if err := c.Spend(ctx, ws, CostDraft, ReasonDraft); err != nil {
			t.Fatalf("spend 5 of 7: %v", err)
		}
		if err := c.Spend(ctx, ws, CostDraft, ReasonDraft); !errors.Is(err, ErrNoCredits) {
			t.Fatalf("spend 5 of 2: %v, want ErrNoCredits", err)
		}
		if b := balance(t, ctx, q, ws); b != 2 {
			t.Fatalf("balance = %d, want 2 (failed spend must not deduct)", b)
		}
	})

	t.Run("grant is idempotent per ref", func(t *testing.T) {
		ws := newWorkspace(t, ctx, pool, q, 0)
		ok, bal, err := c.Grant(ctx, ws, 1000, ReasonTopup, "pay:p_1")
		if err != nil || !ok || bal != 1000 {
			t.Fatalf("first grant = %v %d %v", ok, bal, err)
		}
		ok, bal, err = c.Grant(ctx, ws, 1000, ReasonTopup, "pay:p_1")
		if err != nil || ok || bal != 1000 {
			t.Fatalf("replayed grant = %v %d %v, want no-op at 1000", ok, bal, err)
		}
		if b := balance(t, ctx, q, ws); b != 1000 {
			t.Fatalf("balance = %d", b)
		}
	})

	t.Run("free plan refresh tops up to the allowance after 30 days", func(t *testing.T) {
		low := newWorkspace(t, ctx, pool, q, 12)
		rich := newWorkspace(t, ctx, pool, q, 900)
		fresh := newWorkspace(t, ctx, pool, q, 0)
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET credits_refreshed_at = now() - interval '31 days' WHERE id IN ($1, $2)`, low, rich); err != nil {
			t.Fatal(err)
		}
		if _, err := c.RefreshFree(ctx); err != nil {
			t.Fatal(err)
		}
		if b := balance(t, ctx, q, low); b != 300 {
			t.Errorf("low = %d, want 300", b)
		}
		if b := balance(t, ctx, q, rich); b != 900 {
			t.Errorf("rich = %d, want 900 (never lowered)", b)
		}
		if b := balance(t, ctx, q, fresh); b != 0 {
			t.Errorf("fresh = %d, want 0 (refreshed < 30 days ago)", b)
		}
	})
}
