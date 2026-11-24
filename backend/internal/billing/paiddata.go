package billing

import (
	"context"
	"errors"
	"fmt"

	"leadecho/internal/database"
)

var (
	// ErrNoPaidData: this workspace isn't allowed to spend the operator's treg balance.
	ErrNoPaidData = errors.New("paid data is not enabled for this workspace")
	// ErrPaidDataPaused: the operator switched all paid data off in /ops.
	ErrPaidDataPaused = errors.New("paid data is paused")
)

// PaidPlatforms are the sources searched with paid data. Hosted Free plans
// can't use them; the app shows them locked and the monitor skips them.
var PaidPlatforms = map[string]bool{"twitter": true, "linkedin": true, "reddit": true}

// LockedFor returns the platforms a workspace on plan planID can't search:
// every PaidPlatform without paid data, otherwise the plan's LockedPlatforms.
func LockedFor(planID string, paidData bool) map[string]bool {
	out := map[string]bool{}
	if !paidData {
		for p := range PaidPlatforms {
			out[p] = true
		}
		return out
	}
	plan, _ := PlanByID(planID)
	for _, p := range plan.LockedPlatforms {
		out[p] = true
	}
	return out
}

// LockedPlatforms returns the platforms this workspace can't search (see
// LockedFor). nil on self-host (credits not enforced) and on a lookup error,
// which fails open so monitoring doesn't stop.
func (c *Credits) LockedPlatforms(ctx context.Context, wsID string) map[string]bool {
	if !c.Enforced() {
		return nil
	}
	st, err := c.q.GetBillingState(ctx, wsID)
	if err != nil {
		return nil
	}
	return LockedFor(st.Plan, st.PaidData)
}

// PaidDataPaused reports whether the operator switched paid data off for
// everyone in /ops (hosted only).
func (c *Credits) PaidDataPaused(ctx context.Context) bool {
	if c == nil || c.q == nil || !c.Enforced() {
		return false
	}
	v, err := c.q.GetOpsSetting(ctx, "paid_data_paused")
	return err == nil && string(v) == "true"
}

// PaidDataMeter gates every treg call (it implements treg.Meter): the global
// /ops switch, then the workspace's paid-data access, then one credit.
type PaidDataMeter struct {
	q       *database.Queries
	credits *Credits
}

func NewPaidDataMeter(q *database.Queries, credits *Credits) *PaidDataMeter {
	return &PaidDataMeter{q: q, credits: credits}
}

func (m *PaidDataMeter) Spend(ctx context.Context, wsID string, cost int, reason string) error {
	if m == nil || m.q == nil || !m.credits.Enforced() {
		return nil // self-host / dev: the operator's own keys, no gate
	}
	paused, err := m.q.GetOpsSetting(ctx, "paid_data_paused")
	if err == nil && string(paused) == "true" {
		return ErrPaidDataPaused
	}
	access, err := m.q.GetWorkspaceAccess(ctx, wsID)
	if err != nil {
		return fmt.Errorf("paid data access: %w", err)
	}
	if !access.PaidData {
		return ErrNoPaidData
	}
	return m.credits.Spend(ctx, wsID, cost, reason)
}

func (m *PaidDataMeter) Refund(ctx context.Context, wsID string, n int, reason string) {
	if m != nil {
		m.credits.Refund(ctx, wsID, n, reason)
	}
}

// OverheadMeter gates treg calls the platform pays for itself (shared,
// cached lookups such as subreddit rules): only the global /ops switch, no
// workspace paid-data flag and no credit. It implements treg.Meter.
type OverheadMeter struct{ Credits *Credits }

func (m OverheadMeter) Spend(ctx context.Context, _ string, _ int, _ string) error {
	if m.Credits.PaidDataPaused(ctx) {
		return ErrPaidDataPaused
	}
	return nil
}

func (OverheadMeter) Refund(context.Context, string, int, string) {}

// ScanAllowed: false when /ops paused the workspace's scanning (always
// honoured) or, with credits enforced, when its balance is empty.
func (c *Credits) ScanAllowed(ctx context.Context, wsID string) bool {
	if c == nil || c.q == nil {
		return true
	}
	st, err := c.q.GetBillingState(ctx, wsID)
	if err != nil {
		return true // fail open: a lookup error must not stop monitoring
	}
	if st.ScanningPaused {
		return false
	}
	return !c.Enforced() || st.CreditBalance > 0
}
