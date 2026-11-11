package platform

import (
	"context"
	"time"

	"leadecho/internal/demand"
)

// DemandCheckInterval is how often workspaces are checked for a due run;
// each workspace is grouped at most once a day.
const (
	DemandCheckInterval = time.Hour
	demandRunEvery      = 24 * time.Hour
)

// StartDemand groups each workspace's new asks into demand themes once a
// day. Safe in several processes: the daily run is claimed in the database.
func (s *Shared) StartDemand(ctx context.Context) {
	if s.LLMRouter == nil || !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("demand scheduler disabled")
		return
	}
	svc := &demand.Service{Store: demand.DBStore{Q: s.Queries}, Model: s.LLMRouter, Credits: demand.NoCredits{}, Logger: s.Logger}
	go func() {
		t := time.NewTicker(DemandCheckInterval)
		defer t.Stop()
		for {
			s.runDemand(ctx, svc)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *Shared) runDemand(ctx context.Context, svc *demand.Service) {
	kws, err := s.Queries.ListAllActiveKeywords(ctx)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("demand: list workspaces")
		return
	}
	seen := map[string]bool{}
	for _, kw := range kws {
		ws := kw.WorkspaceID
		if seen[ws] || ctx.Err() != nil {
			continue
		}
		seen[ws] = true
		if due, err := svc.Due(ctx, ws); err != nil || !due {
			if err != nil {
				s.Logger.Warn().Err(err).Str("workspace_id", ws).Msg("demand: check waiting asks")
			}
			continue
		}
		claimed, err := s.Queries.ClaimSettingsRun(ctx, ws, demand.SettingsKey, demandRunEvery)
		if err != nil || !claimed {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		placed, err := svc.Refresh(rctx, ws, time.Now().UTC())
		cancel()
		if err != nil {
			s.Logger.Warn().Err(err).Str("workspace_id", ws).Msg("demand: grouping failed")
			continue
		}
		if placed > 0 {
			s.Logger.Info().Int("asks", placed).Str("workspace_id", ws).Msg("demand: themes updated")
		}
	}
}
