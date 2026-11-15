package monitor

import (
	"context"

	"leadecho/internal/ai"
)

// Auto-drafts: a strong post lead used to wait for someone to click "Draft
// reply" (the best one waited four days). It now arrives with its draft.

// autoDraftMinScore is the score from which a post lead is drafted unasked.
const autoDraftMinScore = 7.0

// AutoDrafter writes a reply draft nobody asked for (*reply.Drafter): free
// for the customer, capped per workspace per day, once per lead.
type AutoDrafter interface {
	AutoDraft(ctx context.Context, wsID, mentionID string) (bool, error)
}

// SetAutoDrafter turns auto-drafts on.
func (m *Monitor) SetAutoDrafter(d AutoDrafter) { m.autoDrafter = d }

// autoDraftable: a lead by the Leads tab's own rule, scored 7 or more, whose
// author is a buyer. An unknown role is not enough: a reply written to a
// candidate or a vendor is worse than none.
func autoDraftable(r *ai.ClassifyResult) bool {
	return r != nil && r.RelevanceScore >= autoDraftMinScore && qualifiesAsLead(r.RelevanceScore, r.Intent) && r.AuthorRole == "buyer"
}

// autoDraft drafts the reply for a post that was just scored (or re-scored)
// as a strong lead. Best effort: the lead stands without it.
func (m *Monitor) autoDraft(ctx context.Context, a mentionAlert, r *ai.ClassifyResult) {
	if m.autoDrafter == nil || !autoDraftable(r) {
		return
	}
	drafted, err := m.autoDrafter.AutoDraft(ctx, a.WorkspaceID, a.ID)
	if err != nil {
		m.logger.Warn().Err(err).Str("mention_id", a.ID).Msg("auto-draft failed")
		return
	}
	if drafted {
		m.logger.Info().Str("mention_id", a.ID).Float64("score", r.RelevanceScore).Msg("auto-draft: reply drafted")
	}
}
