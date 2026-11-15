package monitor

import (
	"encoding/json"
	"time"
)

// Where the daily company scan stands, for the setup checklist.
const (
	CompanyScanNever   = "never"   // not run yet: it runs at the end of the workspace's next scan
	CompanyScanRunning = "running" // claimed, and its report is not stored yet
	CompanyScanDone    = "done"    // ran; NextDue says from when it may run again
	CompanyScanNoFit   = "no_fit"  // the planner found no hiring signal that fits this product
)

// CompanyScanProgress reads workspaces.settings.hiring_signals. nextDue is the
// time from which the next daily run may start (the run itself starts when a
// scan of the workspace next reaches it); zero unless the state is done.
func CompanyScanProgress(raw []byte, now time.Time) (state string, nextDue time.Time) {
	var st hiringState
	if len(raw) == 0 || json.Unmarshal(raw, &st) != nil || st.LastRun.IsZero() {
		return CompanyScanNever, time.Time{}
	}
	if st.Plan != nil && !st.Plan.Applies {
		return CompanyScanNoFit, time.Time{}
	}
	// A run stores its report (stamped with its start) when it ends. A claim
	// without one is a run in progress, or one that ended before it could
	// report (the plan could not be written): after manualScanTimeout it is
	// taken to be over.
	reported := st.LastReport != nil && st.LastReport.At.After(st.LastRun.Add(-time.Minute))
	if !reported && now.Sub(st.LastRun) < manualScanTimeout {
		return CompanyScanRunning, time.Time{}
	}
	return CompanyScanDone, st.LastRun.Add(hiringInterval)
}
