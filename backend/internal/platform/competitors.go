package platform

import (
	"context"
	"runtime/debug"
	"time"

	"leadecho/internal/billing"
	"leadecho/internal/competitors"
)

// CompetitorPollInterval is how often the competitor watcher looks for work.
// Each source is still checked at most once a day; a round only picks up
// what is due.
const CompetitorPollInterval = 10 * time.Minute

// StartCompetitors runs the competitor watcher: it sets up workspaces from
// the competitor names onboarding stored, checks their sources once a day
// and turns what is new into moves.
//
// It has its own ticker instead of riding the monitor tick, so a slow round
// of page fetches never delays lead scanning. Run it in one process only
// (the same one that runs the monitor): rounds are not claimed in the
// database.
func (s *Shared) StartCompetitors(ctx context.Context) {
	if s.LLMRouter == nil || !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("competitor watch disabled")
		return
	}
	svc := competitors.NewService(s.Queries, s.LLMRouter, s.Monitor, billing.New(s.Queries, s.Config.CreditsEnforced()), s.Config.ExaAPIKey, s.Logger)
	go func() {
		t := time.NewTicker(CompetitorPollInterval)
		defer t.Stop()
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.Logger.Error().Interface("panic", r).Str("stack", string(debug.Stack())).Msg("competitors: recovered from panic in poll")
					}
				}()
				svc.Poll(ctx)
			}()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
