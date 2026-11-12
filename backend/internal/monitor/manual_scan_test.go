package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var scanT0 = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

func TestNewManualScanValidatesTheKind(t *testing.T) {
	s, err := NewManualScan(ScanPosts, "ops@example.com", scanT0)
	if err != nil || s.Kind != ScanPosts || s.State != ScanRequested || s.By != "ops@example.com" || !s.RequestedAt.Equal(scanT0) {
		t.Fatalf("scan = %+v, err = %v", s, err)
	}
	if _, err := NewManualScan(ScanHiring, "", scanT0); err != nil {
		t.Fatalf("hiring is a valid kind: %v", err)
	}
	if _, err := NewManualScan("everything", "", scanT0); !errors.Is(err, ErrScanKind) {
		t.Fatalf("unknown kind must be rejected, got %v", err)
	}
}

// One manual scan at a time per workspace; a run that never finished (the
// worker restarted) stops blocking after a while.
func TestManualScanBlocksASecondRequestUntilDoneOrStale(t *testing.T) {
	var none ManualScan
	if !none.CanRequest(scanT0) {
		t.Fatal("no scan yet: a request is allowed")
	}
	for name, tc := range map[string]struct {
		s    ManualScan
		want bool
	}{
		"just requested":   {ManualScan{State: ScanRequested, RequestedAt: scanT0.Add(-time.Minute)}, false},
		"running":          {ManualScan{State: ScanRunning, RequestedAt: scanT0.Add(-10 * time.Minute), StartedAt: scanT0.Add(-9 * time.Minute)}, false},
		"finished":         {ManualScan{State: ScanDone, RequestedAt: scanT0.Add(-time.Hour), FinishedAt: scanT0.Add(-50 * time.Minute)}, true},
		"stale running":    {ManualScan{State: ScanRunning, RequestedAt: scanT0.Add(-3 * time.Hour), StartedAt: scanT0.Add(-3 * time.Hour)}, true},
		"stale, unclaimed": {ManualScan{State: ScanRequested, RequestedAt: scanT0.Add(-3 * time.Hour)}, true},
	} {
		if got := tc.s.CanRequest(scanT0); got != tc.want {
			t.Errorf("%s: CanRequest = %v, want %v", name, got, tc.want)
		}
	}
}

func TestManualScanFinishRecordsTheOutcome(t *testing.T) {
	s := ManualScan{Kind: ScanHiring, State: ScanRunning, StartedAt: scanT0}
	done := s.Finish(scanT0.Add(5*time.Minute), "")
	if done.State != ScanDone || !done.FinishedAt.Equal(scanT0.Add(5*time.Minute)) || done.Note != "" || s.State != ScanRunning {
		t.Fatalf("done = %+v (the original must not change: %+v)", done, s)
	}
	if got := s.Finish(scanT0, "scanning is paused for this workspace").Note; got == "" {
		t.Fatal("a skipped run must say why")
	}
	// A run that could not start because another scan held the workspace
	// goes back to waiting, with its original request time (so it still
	// goes stale an hour after it was asked for).
	if again := s.Requeue(); again.State != ScanRequested || !again.StartedAt.IsZero() || again.Kind != ScanHiring {
		t.Fatalf("requeued = %+v", again)
	}
	// It round-trips through the settings JSON without zero timestamps.
	raw, _ := json.Marshal(ManualScan{Kind: ScanPosts, State: ScanRequested, RequestedAt: scanT0})
	if string(raw) != `{"kind":"posts","state":"requested","requested_at":"2026-11-12T09:00:00Z"}` {
		t.Fatalf("json = %s", raw)
	}
}

// A manual run skips one timer (per-platform cadence for a posts scan, the
// once-a-day claim for a hiring scan) and nothing else: the spend caps are
// checked by the same code as a scheduled scan.
func TestForcedScanSkipsOnlyItsOwnTimer(t *testing.T) {
	ctx := context.Background()
	if forcedScan(ctx) != "" {
		t.Fatal("a normal scan is not forced")
	}
	if got := hiringClaimInterval(ctx); got != hiringInterval {
		t.Fatalf("scheduled hiring scan claims once per %s, got %s", hiringInterval, got)
	}
	if got := hiringClaimInterval(withForcedScan(ctx, ScanHiring)); got != 0 {
		t.Fatalf("a manual hiring scan must not wait for tomorrow, got %s", got)
	}
	// A manual posts scan ends with the usual hiring step, which must keep
	// its daily limit.
	posts := withForcedScan(ctx, ScanPosts)
	if forcedScan(posts) != ScanPosts || hiringClaimInterval(posts) != hiringInterval {
		t.Fatalf("posts scan: forced=%q hiring interval=%s", forcedScan(posts), hiringClaimInterval(posts))
	}
}
