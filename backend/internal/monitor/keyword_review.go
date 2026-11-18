package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"leadecho/internal/keywordjudge"
)

// The keyword judge runs as a manual scan of kind "keywords": an operator
// asks in the ops console, the customer asks on the Keywords page, or the
// weekly loop below asks for every Pro workspace. One path, so it gets the
// same claim, restart recovery and status as every other scan.

// ScanByWeekly is who asked when the weekly loop requests a review.
const ScanByWeekly = "weekly"

const keywordReviewPoll = time.Hour

// SetKeywordReviewer turns the keyword judge on.
func (m *Monitor) SetKeywordReviewer(r keywordjudge.Reviewer) { m.keywordReviewer = r }

// runKeywordReview is the "keywords" manual scan; it returns the note shown
// in the ops console.
func (m *Monitor) runKeywordReview(ctx context.Context, ws, by string) string {
	if m.keywordReviewer == nil {
		return "no AI provider is set up on this server"
	}
	st, err := keywordjudge.Review(ctx, m.q, m.keywordReviewer, ws, by, m.credits.PaidPlatformsLocked(ctx, ws), time.Now().UTC())
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review failed")
		return "the keyword review failed: " + err.Error()
	}
	return fmt.Sprintf("reviewed %d posts and %d leads from the last %d days: %d suggestions", st.Posts, st.Leads, st.Days, len(st.Suggestions))
}

// watchKeywordReviews asks for a review of each Pro workspace once a week.
func (m *Monitor) watchKeywordReviews(ctx context.Context) {
	if m.keywordReviewer == nil {
		return
	}
	t := time.NewTicker(keywordReviewPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.requestDueKeywordReviews(ctx, time.Now().UTC())
		}
	}
}

func (m *Monitor) requestDueKeywordReviews(ctx context.Context, now time.Time) {
	ids, err := m.q.ListWorkspacesDueKeywordReview(ctx, now.Add(-keywordjudge.Interval))
	if err != nil {
		m.logger.Warn().Err(err).Msg("keyword review: list due workspaces")
		return
	}
	for _, ws := range ids {
		scan, _ := NewManualScan(ScanKeywords, ScanByWeekly, now)
		raw, _ := json.Marshal(scan)
		if _, err := m.q.RequestManualScan(ctx, ws, raw, StaleBefore(now)); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review: request")
		}
	}
}
