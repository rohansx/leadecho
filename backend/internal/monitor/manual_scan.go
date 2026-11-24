package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Manual scans: an operator asks, in the ops console, for one workspace to
// be scanned now. The request is written to the workspace's settings and the
// process that runs the monitor (the worker in production, where the API has
// no monitor) picks it up. A manual run uses the same code as a scheduled
// one and skips only its timer.

const (
	ScanPosts     = "posts"     // the keyword crawl
	ScanHiring    = "hiring"    // company hiring signals
	ScanProspects = "prospects" // companies in the customer segments
	ScanRescore   = "rescore"   // score recent posts again with the current rules
	ScanRescreen  = "rescreen"  // check recent company leads again and archive the ones that fail
	ScanKeywords  = "keywords"  // the keyword judge: read what the keywords found, suggest changes
	ScanIdentity  = "identity"  // find who is behind older HN/Dev.to/Bluesky leads
	ScanPerson360 = "person360" // write missing Person360 profiles, use found keys for the paid lookup

	// ScanRescoreLeads scores the posts shown as leads again with today's
	// classifier: verdicts only (no new leads rows, drafts, workflows or
	// emails) and no credits taken.
	ScanRescoreLeads = "rescore_leads"

	// ScanByOnboarding is who asked when a workspace finishes onboarding (the
	// By of an operator's request is their email).
	ScanByOnboarding = "onboarding"

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
	Note        string    `json:"note,omitempty"` // why a run did nothing, or what a re-screen did
	// Restarts is how many times a server restart cut this scan short.
	Restarts int `json:"restarts,omitempty"`
}

func NewManualScan(kind, by string, now time.Time) (ManualScan, error) {
	switch kind {
	case ScanPosts, ScanHiring, ScanProspects, ScanRescore, ScanRescoreLeads, ScanRescreen, ScanKeywords, ScanIdentity, ScanPerson360:
	default:
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

// Recover is what becomes of a scan found "running" when a monitor starts
// up: a run started before this process was cut short by the restart (a
// deploy), so it is asked for again, once. A second cut-short run is marked
// done, so a scan that brings the server down does not run forever.
func (s ManualScan) Recover(processStart, now time.Time) (ManualScan, bool) {
	if s.State != ScanRunning || !s.StartedAt.Before(processStart) {
		return s, false
	}
	if s.Restarts > 0 {
		return s.Finish(now, "a server restart stopped it twice; ask again"), true
	}
	s = s.Requeue()
	s.Restarts++
	return s, true
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

// scanContext is what a requested scan runs under. An operator's request
// skips the scan's own timer. The one made when a workspace finishes
// onboarding does not: it starts the workspace's scheduled scan early, so
// whatever already ran today (the daily company scan, a search that is not
// due yet) is not run a second time.
func scanContext(scan ManualScan) context.Context {
	if scan.By == ScanByOnboarding {
		return context.Background()
	}
	return withForcedScan(context.Background(), scan.Kind)
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
	m.recoverManualScans(ctx, time.Now().UTC())
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

// recoverManualScans puts back the scans a restart cut short: before this,
// one stayed "running" until it went stale an hour later, and never ran.
// ponytail: assumes one monitor process (the worker); with several, one
// starting up would requeue a scan another is still running.
func (m *Monitor) recoverManualScans(ctx context.Context, processStart time.Time) {
	scans, err := m.q.ListManualScans(ctx)
	if err != nil {
		m.logger.Warn().Err(err).Msg("manual scan: list on startup")
		return
	}
	for ws, raw := range scans {
		var scan ManualScan
		var id struct {
			RequestedAt string `json:"requested_at"`
		}
		if json.Unmarshal(raw, &scan) != nil || json.Unmarshal(raw, &id) != nil {
			continue
		}
		if next, changed := scan.Recover(processStart, time.Now().UTC()); changed {
			m.logger.Info().Str("workspace_id", ws).Str("kind", scan.Kind).Str("state", next.State).Msg("manual scan: recovered after a restart")
			m.saveManualScan(ws, next, id.RequestedAt)
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
	ctx, cancel := context.WithTimeout(scanContext(scan), manualScanTimeout)
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
		m.notifyFirstLeads(ctx, ws)
	case ScanProspects:
		if note = m.prospectBlocked(ctx, ws); note != "" {
			return
		}
		if len(m.prospectSegments(ctx, ws)) == 0 {
			note = "no customer segment has criteria and buyer titles to search with"
			return
		}
		if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
			busy = true
			return
		}
		defer m.scanning.Delete(ws)
		m.scanProspects(ctx, ws)
		m.notifyFirstLeads(ctx, ws)
	case ScanRescore:
		if m.llmRouter == nil {
			note = "no AI provider is set up on this server"
			return
		}
		if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
			busy = true
			return
		}
		defer m.scanning.Delete(ws)
		switch done, finished := m.rescore(ctx, ws); {
		case !finished:
			note = fmt.Sprintf("it stopped after %d posts with more to go; ask again to carry on", done)
		case done == 0:
			note = "no recent post was scored under older rules"
		}
	case ScanRescoreLeads:
		if m.llmRouter == nil {
			note = "no AI provider is set up on this server"
			return
		}
		if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
			busy = true
			return
		}
		defer m.scanning.Delete(ws)
		note = m.rescoreLeads(ctx, ws, scan.RequestedAt)
	case ScanRescreen:
		if m.hiringPlanner == nil {
			note = "no AI provider is set up on this server"
			return
		}
		if _, taken := m.scanning.LoadOrStore(ws, struct{}{}); taken {
			busy = true
			return
		}
		defer m.scanning.Delete(ws)
		checked, archived, review, err := m.rescreen(ctx, ws)
		note = fmt.Sprintf("checked %d company leads from the last 30 days and archived %d", checked, archived)
		if review > 0 {
			note += fmt.Sprintf("; %d need review: the screen rejected a whole batch, so none of it was archived", review)
		}
		if err != nil {
			note += "; " + err.Error()
		}
	case ScanKeywords:
		note = m.runKeywordReview(ctx, ws, scan.By)
	case ScanIdentity:
		note = m.queueIdentityBackfill(ctx, ws)
	case ScanPerson360:
		note = m.queuePerson360Backfill(ctx, ws)
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

// identityBackfillBatch is how many leads one identity request queues; ask
// again for the next batch (each lead is marked once tried).
const identityBackfillBatch = 100

// queueIdentityBackfill queues the workspace's Leads Ready leads whose author
// was never identified through the same enrichment as a new lead (identity
// walk, research agent, paid lookup, each under its own caps).
func (m *Monitor) queueIdentityBackfill(ctx context.Context, ws string) string {
	if m.enricher == nil {
		return "lead enrichment is not set up on this server"
	}
	refs, err := m.q.ListLeadsWithoutIdentity(ctx, ws, identityBackfillBatch)
	if err != nil {
		return "could not list the workspace's leads"
	}
	// Mark each lead before queueing it: a second request while this batch
	// runs (up to an hour) or a lead that fails before its lookup is not
	// queued again for a week.
	mark, _ := json.Marshal(map[string]any{"identity_attempt_at": time.Now().UTC()})
	for _, ref := range refs {
		if err := m.q.MergeLeadMetadata(ctx, ws, ref.ID, mark); err != nil {
			return "could not mark the workspace's leads"
		}
		m.enricher.EnrichLeadAsync(ref.ID, ws)
	}
	if len(refs) == 0 {
		return "every Leads Ready author here was already looked up"
	}
	return fmt.Sprintf("queued %d leads to find who wrote them; results appear on each lead within minutes", len(refs))
}

// queuePerson360Backfill re-runs enrichment for leads whose author has no
// Person360 profile yet, or whose found LinkedIn/email was never looked up.
// Enrichment writes the profile at its end; the paid lookup runs only for
// leads that need it and have a key.
func (m *Monitor) queuePerson360Backfill(ctx context.Context, ws string) string {
	if m.enricher == nil {
		return "lead enrichment is not set up on this server"
	}
	refs, err := m.q.ListLeadsForPerson360(ctx, ws, identityBackfillBatch)
	if err != nil {
		return "could not list the workspace's leads"
	}
	mark, _ := json.Marshal(map[string]any{"person360_attempt_at": time.Now().UTC()})
	for _, ref := range refs {
		if err := m.q.MergeLeadMetadata(ctx, ws, ref.ID, mark); err != nil {
			return "could not mark the workspace's leads"
		}
		m.enricher.EnrichLeadAsync(ref.ID, ws)
	}
	if len(refs) == 0 {
		return "every Leads Ready author here already has a profile"
	}
	return fmt.Sprintf("queued %d leads for their Person360 profile; profiles appear within a few minutes", len(refs))
}
