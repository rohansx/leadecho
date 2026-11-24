package monitor

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// rescoreLeadsBatch is how many leads one batch scores; a var so a test can
// cut a run short between batches.
var rescoreLeadsBatch int32 = rescoreBatch

// rescoreTally counts what a leads re-score did with the posts it tried:
// a new verdict (kept, changed or dropped), a provider failure (retried by
// the next request) or a refusal (final, the old verdict stays).
type rescoreTally struct{ verdicts, failed, refused atomic.Int32 }

type rescoreTallyKey struct{}

// tallyOf is the run's tally, nil outside a leads re-score (its methods are
// then no-ops).
func tallyOf(ctx context.Context) *rescoreTally {
	t, _ := ctx.Value(rescoreTallyKey{}).(*rescoreTally)
	return t
}

func (t *rescoreTally) verdict() {
	if t != nil {
		t.verdicts.Add(1)
	}
}

func (t *rescoreTally) failure(refused bool) {
	switch {
	case t == nil:
	case refused:
		t.refused.Add(1)
	default:
		t.failed.Add(1)
	}
}

func (t *rescoreTally) attempted() int32 {
	return t.verdicts.Load() + t.failed.Load() + t.refused.Load()
}

func (t *rescoreTally) String() string {
	s := fmt.Sprintf("%d leads got a new verdict", t.verdicts.Load())
	if n := t.failed.Load(); n > 0 {
		s += fmt.Sprintf(", %d failed at the AI provider (asking again retries only those)", n)
	}
	if n := t.refused.Load(); n > 0 {
		s += fmt.Sprintf(", %d were refused by the model and keep their old verdict", n)
	}
	return s
}

// rescoreLeads scores the workspace's shown post leads again with today's
// classifier (ScanRescoreLeads), a batch at a time, until none scored before
// scoredBefore is left or ctx ends. Every post tried is stamped (scored_at);
// one the provider failed on is stamped with rescore_error and stays due for
// the next request, but is not tried again in this run. A run that stops
// early (cut short, or most of a batch failing: the provider is down) is not
// complete, and the next request carries on from it (ManualScan.ContinueFrom).
// It returns the operator's note and whether the run is complete.
func (m *Monitor) rescoreLeads(ctx context.Context, ws string, scoredBefore time.Time) (string, bool) {
	tally := &rescoreTally{}
	ctx = context.WithValue(withForcedScan(ctx, ScanRescoreLeads), rescoreTallyKey{}, tally)
	// ponytail: the run's tried ids go back to the query; fine for hundreds of
	// leads, a temp table if a workspace ever has tens of thousands.
	tried := []string{}
	for ctx.Err() == nil {
		rows, err := m.q.ListLeadsToRescore(ctx, ws, scoredBefore, tried, rescoreLeadsBatch)
		if err != nil {
			m.logger.Error().Err(err).Str("workspace_id", ws).Msg("rescore leads: list posts")
			return fmt.Sprintf("it stopped, could not list the leads: %s; ask again to carry on", tally), false
		}
		if len(rows) == 0 {
			if tally.attempted() == 0 {
				return "no shown lead was scored before this request", true
			}
			return tally.String(), tally.failed.Load() == 0
		}
		alerts := make([]mentionAlert, 0, len(rows))
		for _, r := range rows {
			tried = append(tried, r.ID)
			alerts = append(alerts, mentionAlert{ID: r.ID, WorkspaceID: ws, Platform: r.Platform, Keyword: r.Keyword,
				Title: r.Title, URL: r.URL, Author: r.Author, Content: r.Content})
		}
		attempted, failed := tally.attempted(), tally.failed.Load()
		m.batchScoreMentions(ctx, ws, alerts)
		attempted, failed = tally.attempted()-attempted, tally.failed.Load()-failed
		m.logger.Info().Str("workspace_id", ws).Int32("attempted", attempted).Int32("failed", failed).Msg("rescore leads: batch scored")
		if failed*2 > attempted {
			m.logger.Warn().Str("workspace_id", ws).Msg("rescore leads: most of a batch failed at the provider, stopping")
			return fmt.Sprintf("it stopped: the AI provider failed on %d of %d leads in a batch. So far %s; ask again to carry on", failed, attempted, tally), false
		}
	}
	return fmt.Sprintf("it stopped with more to go: %s; ask again to carry on", tally), false
}
