package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

// ErrNoCredits: the workspace's balance can't cover the action.
var ErrNoCredits = errors.New("out of credits: top up or upgrade your plan")

// Credits meters a workspace's prepaid balance. A nil *Credits, or one built
// with enforced=false (self-host, dev, billing not configured), never blocks.
type Credits struct {
	q        *database.Queries
	enforced bool
}

func New(q *database.Queries, enforced bool) *Credits {
	return &Credits{q: q, enforced: enforced}
}

// Enforced reports whether balances actually gate work.
func (c *Credits) Enforced() bool { return c != nil && c.enforced && c.q != nil }

// Take takes up to want credits and returns how many it got (0..want). For
// batches (posts to score, searches): do as many as were paid for.
func (c *Credits) Take(ctx context.Context, wsID string, want int, reason string) (int, error) {
	if want <= 0 {
		return 0, nil
	}
	if !c.Enforced() {
		return want, nil
	}
	taken, err := c.q.TakeCredits(ctx, database.TakeCreditsParams{WorkspaceID: wsID, Want: int32(want)})
	if err != nil {
		return 0, fmt.Errorf("take credits: %w", err)
	}
	c.recordUsage(ctx, wsID, reason, int(taken))
	return int(taken), nil
}

// Spend takes exactly cost credits or none (ErrNoCredits).
func (c *Credits) Spend(ctx context.Context, wsID string, cost int, reason string) error {
	if !c.Enforced() || cost <= 0 {
		return nil
	}
	if _, err := c.q.SpendCredits(ctx, database.SpendCreditsParams{WorkspaceID: wsID, Cost: int32(cost)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoCredits
		}
		return fmt.Errorf("spend credits: %w", err)
	}
	c.recordUsage(ctx, wsID, reason, cost)
	return nil
}

// Balance is the current balance; unenforced billing reports MaxInt32 so
// callers never treat a self-hosted workspace as empty.
func (c *Credits) Balance(ctx context.Context, wsID string) (int, error) {
	if !c.Enforced() {
		return math.MaxInt32, nil
	}
	st, err := c.q.GetBillingState(ctx, wsID)
	if err != nil {
		return 0, fmt.Errorf("credit balance: %w", err)
	}
	return int(st.CreditBalance), nil
}

// CanAddKeyword reports whether the workspace may activate one more keyword
// under its plan's limit (always true when billing isn't enforced).
func (c *Credits) CanAddKeyword(ctx context.Context, wsID string) (ok bool, limit int, err error) {
	if !c.Enforced() {
		return true, 0, nil
	}
	st, err := c.q.GetBillingState(ctx, wsID)
	if err != nil {
		return false, 0, fmt.Errorf("keyword limit: %w", err)
	}
	used, err := c.q.CountWorkspaceKeywords(ctx, wsID)
	if err != nil {
		return false, 0, fmt.Errorf("keyword limit: %w", err)
	}
	return used < st.KeywordLimit, int(st.KeywordLimit), nil
}

// KeywordActive: whether a keyword created now should start active (false
// once the plan's limit is reached, or when the check itself fails).
func (c *Credits) KeywordActive(ctx context.Context, wsID string) bool {
	ok, _, err := c.CanAddKeyword(ctx, wsID)
	return err == nil && ok
}

// Refund returns credits taken for work that then failed before doing it.
func (c *Credits) Refund(ctx context.Context, wsID string, n int, reason string) {
	if !c.Enforced() || n <= 0 {
		return
	}
	_, _ = c.q.TakeCredits(ctx, database.TakeCreditsParams{WorkspaceID: wsID, Want: int32(-n)})
	c.recordUsage(ctx, wsID, reason, -n)
}

// Grant adds credits once per ref (a payment, a billing period, an event).
// granted=false means the ref was already applied; balance is current either way.
func (c *Credits) Grant(ctx context.Context, wsID string, n int, reason, ref string) (granted bool, balance int, err error) {
	if c == nil || c.q == nil {
		return false, 0, errors.New("billing not configured")
	}
	bal, err := c.q.GrantCredits(ctx, database.GrantCreditsParams{WorkspaceID: wsID, Delta: int32(n), Reason: reason, Ref: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		st, serr := c.q.GetBillingState(ctx, wsID)
		if serr != nil {
			return false, 0, fmt.Errorf("grant credits: %w", serr)
		}
		return false, int(st.CreditBalance), nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("grant credits: %w", err)
	}
	return true, int(bal), nil
}

// RefreshFree tops free workspaces back up to the free allowance every 30 days.
func (c *Credits) RefreshFree(ctx context.Context) (int64, error) {
	if c == nil || c.q == nil {
		return 0, nil
	}
	free, _ := PlanByID("free")
	return c.q.RefreshFreeCredits(ctx, int32(free.MonthlyCredits))
}

func (c *Credits) recordUsage(ctx context.Context, wsID, reason string, n int) {
	if n == 0 {
		return
	}
	// Best effort: the balance is already correct; usage is for display.
	_ = c.q.AddCreditUsage(ctx, database.AddCreditUsageParams{WorkspaceID: wsID, Reason: reason, Credits: int32(n)})
}

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }
