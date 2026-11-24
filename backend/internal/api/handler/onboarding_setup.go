package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/billing"
	"leadecho/internal/cadence"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

// Setup status: what a workspace without a lead is waiting for, computed from
// what is stored. Shown as a checklist on onboarding step 4 and in the inbox.

const (
	stageDone        = "done"
	stageRunning     = "running"
	stageWaiting     = "waiting"
	stageNotIncluded = "not_included"
)

// setupStage is one line of the checklist. Only the facts of its own stage
// are set.
type setupStage struct {
	Key   string `json:"key"`   // posts | scoring | paid_sources | company_leads
	State string `json:"state"` // done | running | waiting | not_included
	// Why it is waiting or not included: no_keywords, plan, not_selected,
	// server, no_fit, paused, paid_paused, out_of_credits, queued, retry.
	Reason   string `json:"reason,omitempty"`
	Posts    *int   `json:"posts,omitempty"`    // posts: collected so far
	Scored   *int   `json:"scored,omitempty"`   // scoring
	Unscored *int   `json:"unscored,omitempty"` // scoring
	// NextAt is a time the schedule gives: the next search of a paid source
	// (its last search plus its cadence), or from when the daily company scan
	// may run again.
	NextAt *time.Time `json:"next_at,omitempty"`
	// EveryMinutes is given when no time can be known: the stage starts with
	// the next scan round, and rounds start this often (a long round delays
	// the next).
	EveryMinutes int `json:"every_minutes,omitempty"`
}

type setupStatus struct {
	Stages []setupStage `json:"stages"`
	Leads  struct {
		Post    int `json:"post"`
		Company int `json:"company"`
	} `json:"leads"`
	HasLead bool `json:"has_lead"`
	// FinishedEmpty: every search has run once, every post is scored, the
	// company scan (when the plan has one) has run, and there is no lead.
	FinishedEmpty bool `json:"finished_empty"`
	// Blocked: nothing runs for this workspace: "paused" by an operator or "out_of_credits".
	Blocked string `json:"blocked,omitempty"`
	// EmailWhenReady: the first-leads email will be sent to this workspace.
	EmailWhenReady bool `json:"email_when_ready"`
}

// setupFacts is everything the status is computed from.
type setupFacts struct {
	Counts database.SetupCounts
	// Searches are (active keyword, platform) pairs the plan includes;
	// Searched are those that have run at least once. Paid* is the subset on
	// X, LinkedIn and Reddit.
	Searches, Searched         int
	PaidSearches, PaidSearched int
	PaidNextAt                 time.Time       // earliest next search of a paid source; zero when unknown
	Locked                     map[string]bool // platforms the plan can't search (billing.LockedFor)
	PaidPaused                 bool            // an operator paused paid data for everyone
	Blocked                    string
	// Scanning: a scan of the workspace is running or asked for. Only known
	// when this process runs the monitor or a requested scan is stored.
	Scanning       bool
	Queued         bool   // a requested scan is stored and not started
	CompanyEnabled bool   // this server has company signals set up
	CompanyScan    string // monitor.CompanyScan*
	CompanyNextDue time.Time
	EmailWhenReady bool
}

func buildSetupStatus(f setupFacts) setupStatus {
	n := func(v int) *int { return &v }
	at := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	everyMinutes := int(monitor.TickInterval / time.Minute)
	// notStarted is a stage that begins with the next scan of the workspace.
	notStarted := func(key string) setupStage {
		switch {
		case f.Blocked != "":
			return setupStage{Key: key, State: stageWaiting, Reason: f.Blocked}
		case f.Queued:
			return setupStage{Key: key, State: stageWaiting, Reason: "queued"}
		case f.Scanning:
			return setupStage{Key: key, State: stageRunning}
		}
		return setupStage{Key: key, State: stageWaiting, EveryMinutes: everyMinutes}
	}

	posts := setupStage{Key: "posts", State: stageDone}
	switch {
	case f.Searches == 0:
		posts = setupStage{Key: "posts", State: stageWaiting, Reason: "no_keywords"}
	case f.Searched == 0:
		posts = notStarted("posts")
	case f.Searched < f.Searches:
		posts.State = stageRunning
		if f.Blocked != "" {
			posts.State, posts.Reason = stageWaiting, f.Blocked
		}
	}
	posts.Posts = n(f.Counts.Posts)
	postsDone := posts.State == stageDone

	scoring := setupStage{Key: "scoring", State: stageWaiting}
	switch {
	case f.Counts.Unscored > 0 && f.Blocked != "":
		scoring.Reason = f.Blocked
	case f.Counts.Unscored > 0:
		scoring.State = stageRunning
	case postsDone:
		scoring.State = stageDone
	}
	scoring.Scored, scoring.Unscored = n(f.Counts.Posts-f.Counts.Unscored), n(f.Counts.Unscored)

	paid := setupStage{Key: "paid_sources", State: stageDone, NextAt: at(f.PaidNextAt)}
	switch {
	case allPaidLocked(f.Locked):
		paid = setupStage{Key: "paid_sources", State: stageNotIncluded, Reason: "plan"}
	case f.PaidSearches == 0:
		paid = setupStage{Key: "paid_sources", State: stageNotIncluded, Reason: "not_selected"}
	case f.PaidPaused:
		// Searches are recorded as run while paid data is paused for everyone,
		// though the provider was never called.
		paid = setupStage{Key: "paid_sources", State: stageWaiting, Reason: "paid_paused"}
	case f.PaidSearched < f.PaidSearches:
		paid = notStarted("paid_sources")
	}

	company := setupStage{Key: "company_leads", State: stageDone, NextAt: at(f.CompanyNextDue)}
	switch {
	case !f.CompanyEnabled:
		company = setupStage{Key: "company_leads", State: stageNotIncluded, Reason: "server"}
	case f.Locked["linkedin"]: // company leads are found with LinkedIn data
		company = setupStage{Key: "company_leads", State: stageNotIncluded, Reason: "plan"}
	case f.CompanyScan == monitor.CompanyScanNoFit:
		company = setupStage{Key: "company_leads", State: stageNotIncluded, Reason: "no_fit"}
	case f.CompanyScan == monitor.CompanyScanRunning:
		company = setupStage{Key: "company_leads", State: stageRunning}
	case f.CompanyScan != monitor.CompanyScanDone && f.PaidPaused:
		company = setupStage{Key: "company_leads", State: stageWaiting, Reason: "paused"}
	case f.CompanyScan == monitor.CompanyScanRetry && f.Blocked == "":
		// The last run could not do its work (a provider or the model failed)
		// and is tried again from NextAt: not "ran, found nothing".
		company = setupStage{Key: "company_leads", State: stageWaiting, Reason: "retry", NextAt: at(f.CompanyNextDue)}
	case f.CompanyScan != monitor.CompanyScanDone:
		company = notStarted("company_leads")
	}

	s := setupStatus{Stages: []setupStage{posts, scoring, paid, company}, Blocked: f.Blocked, EmailWhenReady: f.EmailWhenReady}
	s.Leads.Post, s.Leads.Company = f.Counts.PostLeads, f.Counts.CompanyLeads
	s.HasLead = s.Leads.Post+s.Leads.Company > 0
	settled := func(st setupStage) bool { return st.State == stageDone || st.State == stageNotIncluded }
	s.FinishedEmpty = !s.HasLead && postsDone && scoring.State == stageDone && settled(paid) && settled(company)
	return s
}

// allPaidLocked: the plan includes none of the paid platforms.
func allPaidLocked(locked map[string]bool) bool {
	for p := range billing.PaidPlatforms {
		if !locked[p] {
			return false
		}
	}
	return true
}

// searchProgress counts the workspace's searches (active keyword x platform)
// and how many have run, leaving out the paid platforms when the plan has
// none. paidNextAt is the earliest time a paid search is next due: its last
// run plus its cadence.
// ponytail: a keyword slowed down for bringing posts and no leads is searched
// once a day whatever its cadence; that is not reflected here (the paid
// default is already once a day).
func searchProgress(kws []database.ListKeywordCadencesRow, scans []database.ListKeywordPlatformScansRow, locked map[string]bool) (searches, searched, paid, paidSearched int, paidNextAt time.Time) {
	last := map[[2]string]time.Time{}
	for _, s := range scans {
		last[[2]string{s.KeywordID, s.Platform}] = s.LastScannedAt
	}
	for _, k := range kws {
		if !k.IsActive {
			continue
		}
		cad, err := cadence.Parse(k.Cadence)
		if err != nil {
			cad = cadence.Map{}
		}
		for _, p := range k.Platforms {
			isPaid := billing.PaidPlatforms[p]
			if locked[p] {
				continue
			}
			t, ran := last[[2]string{k.ID, p}]
			searches++
			if ran {
				searched++
			}
			if !isPaid {
				continue
			}
			paid++
			if ran {
				paidSearched++
				if next := t.Add(cad.Interval(p)); paidNextAt.IsZero() || next.Before(paidNextAt) {
					paidNextAt = next
				}
			}
		}
	}
	return
}

// SetSetupFacts tells the setup status what this server can do: find company
// leads (an AI provider and the data provider are set up) and send email.
func (h *OnboardingHandler) SetSetupFacts(companyLeads, email bool) {
	h.companyLeads, h.emailOn = companyLeads, email
}

func (h *OnboardingHandler) setupFacts(ctx context.Context, wsID string) (setupFacts, error) {
	now := time.Now().UTC()
	f := setupFacts{CompanyEnabled: h.companyLeads}
	ws, err := h.q.GetWorkspace(ctx, wsID)
	if err != nil {
		return f, err
	}
	if f.Counts, err = h.q.CountSetup(ctx, wsID); err != nil {
		return f, err
	}
	kws, err := h.q.ListKeywordCadences(ctx, wsID)
	if err != nil {
		return f, err
	}
	scans, err := h.q.ListKeywordPlatformScans(ctx, wsID)
	if err != nil {
		return f, err
	}
	f.Locked, f.PaidPaused = h.credits.LockedPlatforms(ctx, wsID), h.credits.PaidDataPaused(ctx)
	f.Searches, f.Searched, f.PaidSearches, f.PaidSearched, f.PaidNextAt = searchProgress(kws, scans, f.Locked)
	if bs, err := h.q.GetBillingState(ctx, wsID); err == nil { // the same reading as billing.Credits.ScanAllowed
		switch {
		case bs.ScanningPaused:
			f.Blocked = "paused"
		case h.credits.Enforced() && bs.CreditBalance <= 0:
			f.Blocked = "out_of_credits"
		}
	}
	var settings struct {
		Hiring json.RawMessage    `json:"hiring_signals"`
		Manual monitor.ManualScan `json:"manual_scan"`
	}
	_ = json.Unmarshal(ws.Settings, &settings) // unreadable settings: nothing has run
	f.CompanyScan, f.CompanyNextDue = monitor.CompanyScanProgress(settings.Hiring, now)
	asked := settings.Manual.Kind == monitor.ScanPosts || settings.Manual.Kind == monitor.ScanHiring
	if asked && !settings.Manual.CanRequest(now) { // waiting or running, and not abandoned
		f.Queued = settings.Manual.State == monitor.ScanRequested
		f.Scanning = !f.Queued
	}
	if bs, ok := h.scanner.(BackfillScanner); ok && bs.IsScanning(wsID) {
		f.Scanning = true
	}
	f.EmailWhenReady = h.emailOn && monitor.FirstLeadsEmailTo(ctx, h.q, ws, f.Counts.OldestLead, now) != ""
	return f, nil
}

// SetupStatus returns the checklist for a workspace on its way to a first lead.
// GET /onboarding/setup-status
func (h *OnboardingHandler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	f, err := h.setupFacts(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load setup status")
		return
	}
	writeJSON(w, http.StatusOK, buildSetupStatus(f))
}

// requestFirstScan asks the process that runs the monitor to start this
// workspace's scheduled scan now instead of when its round reaches it. That
// scan ends with the daily company scan, which is why this is only asked for
// a workspace the company scan would run for: one that may scan and has paid
// data. The scan itself applies every gate again, with the plan's budgets.
// It is not the operator's "scan now": nothing that already ran today runs
// twice (monitor.ScanByOnboarding). A request already waiting is kept.
func (h *OnboardingHandler) requestFirstScan(ctx context.Context, wsID string) {
	if !h.credits.ScanAllowed(ctx, wsID) || h.credits.LockedPlatforms(ctx, wsID)["linkedin"] || h.credits.PaidDataPaused(ctx) {
		return
	}
	now := time.Now().UTC()
	scan, _ := monitor.NewManualScan(monitor.ScanPosts, monitor.ScanByOnboarding, now)
	raw, _ := json.Marshal(scan)
	if _, err := h.q.RequestManualScan(ctx, wsID, raw, monitor.StaleBefore(now)); err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: request first scan")
	}
}
