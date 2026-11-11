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

// PaidPlatformsLocked reports whether this workspace can't use PaidPlatforms:
// hosted (credits enforced) with paid data off. Self-host is never locked,
// and a lookup error fails open so monitoring doesn't stop.
func (c *Credits) PaidPlatformsLocked(ctx context.Context, wsID string) bool {
	if !c.Enforced() {
		return false
	}
	access, err := c.q.GetWorkspaceAccess(ctx, wsID)
	return err == nil && !access.PaidData
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
