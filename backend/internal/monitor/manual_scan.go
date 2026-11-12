package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Manual scans: an operator asks, in the ops console, for one workspace to
// be scanned now. The request is written to the workspace's settings and the
// process that runs the monitor (the worker in production, where the API has
// no monitor) picks it up. A manual run uses the same code as a scheduled
// one and skips only its timer.

const (
	ScanPosts  = "posts"  // the keyword crawl
	ScanHiring = "hiring" // company hiring signals

	ScanRequested = "requested"
	ScanRunning   = "running"
	ScanDone      = "done"

	// manualScanStale: a request or run older than this is treated as
	// abandoned (the worker restarted mid-run) and no longer blocks a new one.
	manualScanStale   = time.Hour
	manualScanPoll    = 20 * time.Second
	manualScanTimeout = 45 * time.Minute
)

var ErrScanKind = errors.New("unknown scan kind")

// ManualScan is stored under workspaces.settings.manual_scan.
type ManualScan struct {
	Kind        string    `json:"kind"`
	State       string    `json:"state"`
	By          string    `json:"by,omitempty"`
	RequestedAt time.Time `json:"requested_at,omitzero"`
	StartedAt   time.Time `json:"started_at,omitzero"`
	FinishedAt  time.Time `json:"finished_at,omitzero"`
	Note        string    `json:"note,omitempty"` // why a run did nothing
}

func NewManualScan(kind, by string, now time.Time) (ManualScan, error) {
	if kind != ScanPosts && kind != ScanHiring {
		return ManualScan{}, ErrScanKind
	}
	return ManualScan{Kind: kind, State: ScanRequested, By: by, RequestedAt: now}, nil
}

// CanRequest reports whether a new manual scan may be asked for: not while
// one is waiting or running, unless that one is stale.
func (s ManualScan) CanRequest(now time.Time) bool {
	switch s.State {
	case ScanRequested:
		return now.Sub(s.RequestedAt) > manualScanStale
	case ScanRunning:
		return now.Sub(s.StartedAt) > manualScanStale
	}
	return true
}

// StaleBefore is the time before which a waiting or running scan counts as
// abandoned.
func StaleBefore(now time.Time) time.Time { return now.Add(-manualScanStale) }

// Requeue returns the scan waiting again: it was claimed but another scan
// held the workspace. The request time is kept, so it still goes stale.
func (s ManualScan) Requeue() ManualScan {
	s.State, s.StartedAt = ScanRequested, time.Time{}
	return s
}

// Finish returns the scan marked done; note says why it did nothing, if so.
func (s ManualScan) Finish(now time.Time, note string) ManualScan {
	s.State, s.FinishedAt, s.Note = ScanDone, now, note
	return s
}

type forcedScanKey struct{}

// withForcedScan marks a scan as manual, so its own timer is skipped.
func withForcedScan(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, forcedScanKey{}, kind)
}

// forcedScan is the kind of manual scan running, "" for a scheduled one.
func forcedScan(ctx context.Context) string {
	kind, _ := ctx.Value(forcedScanKey{}).(string)
	return kind
}

// hiringClaimInterval is how long after the last hiring scan the next may
// run: a day, or at once when an operator asked for a hiring scan.
func hiringClaimInterval(ctx context.Context) time.Duration {
	if forcedScan(ctx) == ScanHiring {
		return 0
	}
	return hiringInterval
}

// watchManualScans runs requested scans until ctx ends. It is its own loop
// because a tick over every workspace can take many minutes.
func (m *Monitor) watchManualScans(ctx context.Context) {
	t := time.NewTicker(manualScanPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.startManualScans(ctx)
		}
	}
}

func (m *Monitor) startManualScans(ctx context.Context) {
	ids, err := m.q.ListRequestedManualScans(ctx)
	if err != nil {
		m.logger.Warn().Err(err).Msg("manual scan: list requests")
		return
	}
	for _, ws := range ids {
		if m.IsScanning(ws) {
			continue // a scan of this workspace is running: start after it
		}
		// The claim is atomic, so with several monitor processes only one runs it.
		raw, ok, err := m.q.ClaimManualScan(ctx, ws, time.Now().UTC())
		if err != nil || !ok {
			continue
		}
		var scan ManualScan
		var id struct {
			RequestedAt string `json:"requested_at"` // exactly as stored: it identifies this request
		}
		if json.Unmarshal(raw, &scan) != nil || json.Unmarshal(raw, &id) != nil {
			m.logger.Warn().Str("workspace_id", ws).Msg("manual scan: unreadable request, dropped")
			m.saveManualScan(ws, ManualScan{State: ScanDone, FinishedAt: time.Now().UTC(), Note: "the request could not be read"}, "")
			continue
		}
		go m.runManualScan(ws, scan, id.RequestedAt)
	}
}

func (m *Monitor) runManualScan(ws string, scan ManualScan, requestID string) {
	ctx, cancel := context.WithTimeout(withForcedScan(context.Background(), scan.Kind), manualScanTimeout)
	defer cancel()
	note, busy := "", false
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error().Interface("panic", r).Str("workspace_id", ws).Msg("manual scan: recovered from panic")
			note, busy = "the scan failed; see the server log", false
		}
		if busy { // another scan held the workspace: wait for it instead of dropping the request
			m.saveManualScan(ws, scan.Requeue(), requestID)
			return
		}
		m.saveManualScan(ws, scan.Finish(time.Now().UTC(), note), requestID)
	}()
	m.logger.Info().Str("workspace_id", ws).Str("kind", scan.Kind).Str("by", scan.By).Msg("manual scan: started")
	// The same gate as a scheduled scan: paused in the ops console, or out of credits.
	if !m.credits.ScanAllowed(ctx, ws) {
		note = "scanning is paused for this workspace, or it is out of credits"
		return
	}
	switch scan.Kind {
	case ScanHiring:
		if note = m.hiringBlocked(ctx, ws); note != "" {
			return
		}
		if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
			busy = true
			return
		}
		defer m.scanning.Delete(ws)
		m.scanHiring(ctx, ws)
	default:
		kws, err := m.q.ListActiveKeywords(ctx, ws)
		switch {
		case err != nil:
			note = "could not load the workspace's keywords"
		case len(kws) == 0:
			note = "the workspace has no active keywords"
		case !m.scanWorkspace(ctx, ws, kws):
			busy = true // allowed (checked above), so the only reason is a scan already running
		}
	}
}

// saveManualScan stores a manual scan's new state, unless a newer request
// has replaced it meanwhile (requestID "" = whatever is stored).
func (m *Monitor) saveManualScan(ws string, scan ManualScan, requestID string) {
	raw, _ := json.Marshal(scan)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.q.SaveManualScan(ctx, ws, raw, requestID); err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", ws).Msg("manual scan: save state")
	}
}

// hiringBlocked says, in the operator's terms, why a hiring scan cannot run
// ("" when it can). It mirrors the checks at the top of scanHiring.
func (m *Monitor) hiringBlocked(ctx context.Context, ws string) string {
	switch {
	case m.hiringPlanner == nil || !m.treg.Enabled():
		return "hiring signals are not set up on this server"
	case m.credits.PaidDataPaused(ctx):
		return "paid data is paused for everyone"
	case m.credits.PaidPlatformsLocked(ctx, ws):
		return "paid data is off for this workspace"
	}
	if bp, err := m.q.GetBrandProfile(ctx, ws); err != nil || bp.Product == "" {
		return "the workspace has not finished onboarding"
	}
	return ""
}
