package monitor

import (
	"context"
	"time"

	"leadecho/internal/keywordjudge"
)

// Re-scoring: when the scoring rules change, posts already scored keep their
// old verdict, so a buyer the old rules missed stays buried. An operator can
// ask for a workspace's recent posts to be scored again; a keyword review
// that changes the workspace's learned rules does it on its own.

const (
	// ScoringRubric is stamped on every scored post. Raise it when the filter
	// or classifier prompts change in a way that should move old scores: a
	// re-score then picks up exactly the posts scored under older rules, and
	// one that was cut short resumes where it stopped.
	// 3: a buyer's first-hand account of the problem counts as a lead whatever
	// its form; a buyer's success story is a proof point (low score, ask kept).
	// 4: a buyer's own current struggle is not "partly solves" (6+).
	ScoringRubric = 4

	rescoreWindow = 30 * 24 * time.Hour
	rescoreBatch  = 100
)

// rescore scores the workspace's recent posts again, a batch at a time,
// until none are left under older rules or ctx ends. It returns how many
// posts it went through and whether it got through all of them; a run that
// was cut short carries on from where it stopped when asked again. The
// customer is not charged credits for it.
func (m *Monitor) rescore(ctx context.Context, ws string) (done int, finished bool) {
	since := time.Now().Add(-rescoreWindow)
	// Posts scored under an older set of the workspace's learned rules are
	// due too (only classified ones: the rules change classification).
	stamp := ""
	if lr, err := keywordjudge.LoadRules(ctx, m.q, ws); err == nil {
		stamp = keywordjudge.RulesStamp(lr.Texts())
	}
	for ctx.Err() == nil {
		rows, err := m.q.ListMentionsToRescore(ctx, ws, since, ScoringRubric, stamp, rescoreBatch)
		if err != nil {
			m.logger.Error().Err(err).Str("workspace_id", ws).Msg("rescore: list posts")
			break
		}
		if len(rows) == 0 {
			return done, true
		}
		alerts := make([]mentionAlert, len(rows))
		for i, r := range rows {
			alerts[i] = mentionAlert{ID: r.ID, WorkspaceID: ws, Platform: r.Platform, Keyword: r.Keyword,
				Title: r.Title, URL: r.URL, Author: r.Author, Content: r.Content}
		}
		m.batchScoreMentions(ctx, ws, alerts)
		// A post whose second classification failed keeps its old verdict and
		// is listed again: stop when a batch moves nothing (the model is down).
		left, err := m.q.ListMentionsToRescore(ctx, ws, since, ScoringRubric, stamp, rescoreBatch)
		if err == nil && len(left) == len(rows) && left[0].ID == rows[0].ID {
			m.logger.Warn().Str("workspace_id", ws).Msg("rescore: a batch changed nothing, stopping")
			break
		}
		done += len(rows)
		m.logger.Info().Str("workspace_id", ws).Int("done", done).Msg("rescore: batch scored")
	}
	return done, false
}
