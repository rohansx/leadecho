package platform

import (
	"context"
	"runtime/debug"
	"time"

	"leadecho/internal/billing"
	"leadecho/internal/competitors"
	"leadecho/internal/database"
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
// of page fetches never delays lead scanning. Safe in several processes:
// a round runs under a database advisory lock, so only one runs at a time.
func (s *Shared) StartCompetitors(ctx context.Context) {
	if s.LLMRouter == nil || !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("competitor watch disabled")
		return
	}
	svc := competitors.NewService(s.DB, s.Queries, s.LLMRouter, s.Monitor, billing.New(s.Queries, s.Config.CreditsEnforced()), s.Config.ExaAPIKey, s.Logger)
	svc.Active = func(ctx context.Context, ws string) bool {
		return s.featureInUse(ctx, ws, database.FeatureCompetitors)
	}
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
				s.pollCompetitorsOnce(ctx, svc)
			}()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// competitorPollLock is the advisory lock key for a poll round ("cwatch").
const competitorPollLock int64 = 0x637761746368

// pollCompetitorsOnce runs one round unless another process is already
// running one: two rounds at once would fetch, pay and alert twice.
func (s *Shared) pollCompetitorsOnce(ctx context.Context, svc *competitors.Service) {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("competitors: acquire connection")
		return
	}
	defer conn.Release()
	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", competitorPollLock).Scan(&got); err != nil || !got {
		if err != nil {
			s.Logger.Warn().Err(err).Msg("competitors: poll lock")
		}
		return
	}
	// Unlock on a fresh context: ctx may already be cancelled at shutdown.
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", competitorPollLock) //nolint:errcheck
	svc.Poll(ctx)
}
