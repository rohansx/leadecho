package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"leadecho/internal/ai"
	"leadecho/internal/database"
	"time"

	"leadecho/internal/keywordjudge"
)

// The keyword judge runs as a manual scan of kind "keywords": an operator
// asks in the ops console, the customer asks on the Keywords page, or the
// weekly loop below asks for every Pro workspace. One path, so it gets the
// same claim, restart recovery and status as every other scan.

// ScanByWeekly is who asked when the weekly loop requests a review.
const ScanByWeekly = "weekly"

// KeywordReviewFailed is the note of a failed review (the detail is in the
// server log, not shown to the customer).
const KeywordReviewFailed = "the keyword review failed; try again later"

const keywordReviewPoll = time.Hour

// keywordReviewsPerTick staggers the weekly reviews (the first deploy finds
// every Pro workspace due at once).
const keywordReviewsPerTick = 3

// SetKeywordReviewer turns the keyword judge on.
func (m *Monitor) SetKeywordReviewer(r keywordjudge.KeywordAI) { m.keywordReviewer = r }

// runKeywordReview is the "keywords" manual scan; it returns the note shown
// in the ops console.
func (m *Monitor) runKeywordReview(ctx context.Context, ws, by string) string {
	if m.keywordReviewer == nil {
		return "no AI provider is set up on this server"
	}
	var rs keywordjudge.Rescorer
	if m.llmRouter != nil {
		rs = m
	}
	// 1. A Pro workspace under the target gets keywords written and added,
	// and every keyword is searched now, so the review reads fresh posts.
	topUp := m.topUpKeywords(ctx, ws)
	now := time.Now().UTC()
	st, err := keywordjudge.Review(ctx, m.q, m.keywordReviewer, rs, ws, by, m.credits.PaidPlatformsLocked(ctx, ws), now)
	if st != nil && err != nil { // the review was saved; only the rules step failed
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review: scoring rules")
		err = nil
	}
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review failed")
		return KeywordReviewFailed
	}
	note := topUp + fmt.Sprintf("reviewed %d posts and %d leads from the last %d days: %d suggestions", st.Posts, st.Leads, st.Days, len(st.Suggestions))
	lr, err := keywordjudge.LoadRules(ctx, m.q, ws)
	if err != nil || lr.LastTest == nil || !lr.LastTest.At.Equal(now) {
		return note // no new rules proposed this time
	}
	if !lr.LastTest.Applied {
		return note + "; new scoring rules " + lr.LastTest.Note
	}
	return note + "; new scoring rules applied; " + m.rescoreAfterRules(ctx, ws)
}

// rescoreAfterRules scores the workspace's recent posts again under its new
// rules, so the inbox shows what they change now rather than for new posts
// only. Same run as an operator's "Re-score": no customer credits.
func (m *Monitor) rescoreAfterRules(ctx context.Context, ws string) string {
	if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
		return "a scan is running, so posts were not re-scored; ask for a Re-score"
	}
	defer m.scanning.Delete(ws)
	done, finished := m.rescore(withForcedScan(ctx, ScanRescore), ws)
	if !finished {
		return fmt.Sprintf("re-scored %d posts, more to go; ask for a Re-score to carry on", done)
	}
	return fmt.Sprintf("re-scored %d posts", done)
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
	ids, err := m.q.ListWorkspacesDueKeywordReview(ctx, now.Add(-keywordjudge.Interval), keywordReviewsPerTick)
	if err != nil {
		m.logger.Warn().Err(err).Msg("keyword review: list due workspaces")
		return
	}
	for _, ws := range ids {
		// Marked first: a review that fails, or is not allowed (out of
		// credits), waits a week like a successful one.
		if err := m.q.MarkKeywordReviewAttempt(ctx, ws, now); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review: mark attempt")
			continue
		}
		scan, _ := NewManualScan(ScanKeywords, ScanByWeekly, now)
		raw, _ := json.Marshal(scan)
		if _, err := m.q.RequestManualScan(ctx, ws, raw, StaleBefore(now)); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword review: request")
		}
	}
}

// RescoreWith scores a stored post again with candidate scoring rules and
// examples, without storing anything, and says whether it would be a lead.
// It is how the keyword judge tests rules before they are used.
func (m *Monitor) RescoreWith(ctx context.Context, wsID, mentionID string, rules []string, examples []ai.ScoringExample) (bool, error) {
	mn, err := m.q.GetMention(ctx, database.GetMentionParams{ID: mentionID, WorkspaceID: wsID})
	if err != nil {
		return false, fmt.Errorf("load mention: %w", err)
	}
	sc := scoringContextWith(ctx, m.q, m.extraExamples, wsID, mentionID, &examplesOverride{rules: rules, examples: examples}, func(error) {})
	res, err := m.llmRouter.ClassifyIntent(ctx, wsID, mn.Title.String, mn.Content, string(mn.Platform), sc)
	if err != nil {
		return false, err
	}
	return qualifiesAsLead(res.RelevanceScore, res.Intent), nil
}

// topUpKeywords brings a Pro workspace (any workspace on self-host) up to
// keywordjudge.TargetKeywords active keywords, within its plan limit, with
// keywords the AI writes from the product profile, then searches every
// keyword now. It returns a note ("" when nothing was added).
func (m *Monitor) topUpKeywords(ctx context.Context, ws string) string {
	bs, err := m.q.GetBillingState(ctx, ws)
	if err != nil || (m.credits.Enforced() && bs.Plan != "pro") {
		return ""
	}
	kws, err := m.q.ListKeywords(ctx, ws)
	if err != nil {
		return ""
	}
	active := 0
	for _, k := range kws {
		if k.IsActive {
			active++
		}
	}
	limit := int(bs.KeywordLimit)
	if !m.credits.Enforced() || limit <= 0 {
		limit = keywordjudge.TargetKeywords
	}
	want := min(keywordjudge.TargetKeywords-active, limit-active, 25)
	if want <= 0 {
		return ""
	}
	profiles, err := m.q.ListActiveMonitoringProfiles(ctx, ws)
	if err != nil || len(profiles) == 0 {
		return ""
	}
	sugg, err := keywordjudge.Suggest(ctx, m.q, m.keywordReviewer, ws, m.credits.PaidPlatformsLocked(ctx, ws), want)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("keyword top-up: suggest")
		return "could not write new keywords; "
	}
	added := 0
	for _, s := range sugg {
		if len(s.Platforms) == 0 {
			continue
		}
		if _, err := m.q.CreateKeyword(ctx, database.CreateKeywordParams{
			WorkspaceID: ws, ProfileID: profiles[0].ID, Term: s.Term, Platforms: s.Platforms,
			IsActive: true, MatchType: "contains", NegativeTerms: []string{}, Subreddits: []string{},
		}); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", ws).Str("term", s.Term).Msg("keyword top-up: create")
			continue
		}
		added++
	}
	if added == 0 {
		return ""
	}
	m.logger.Info().Str("workspace_id", ws).Int("added", added).Msg("keyword top-up: keywords added")
	note := fmt.Sprintf("added %d keywords; ", added)
	if all, err := m.q.ListActiveKeywords(ctx, ws); err == nil && len(all) > 0 {
		if m.scanWorkspace(withForcedScan(ctx, ScanPosts), ws, all) {
			note += "searched every keyword; "
		}
	}
	return note
}
