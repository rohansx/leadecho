package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/keywordjudge"
	"leadecho/internal/monitor"
)

// KeywordJudgeHandler serves the Keywords page's two AI helpers: "generate
// more keywords" (the Pro top-up to 30) and the keyword judge's report.
type KeywordJudgeHandler struct {
	q         *database.Queries
	credits   *billing.Credits
	suggester keywordjudge.Suggester

	mu       sync.Mutex
	lastCall map[string]time.Time // workspace -> last suggest call
}

func NewKeywordJudgeHandler(q *database.Queries, credits *billing.Credits, s keywordjudge.Suggester) *KeywordJudgeHandler {
	return &KeywordJudgeHandler{q: q, credits: credits, suggester: s, lastCall: map[string]time.Time{}}
}

const (
	// suggestEvery: one strong-model call per workspace per this long.
	suggestEvery = 20 * time.Second
	maxSuggest   = 40
	// reviewEvery: the customer can ask for a fresh review this often;
	// retryEvery: or this long after one that failed.
	reviewEvery = 6 * time.Hour
	retryEvery  = 30 * time.Minute
)

type keywordReviewState struct {
	Review         *keywordjudge.Stored `json:"review"`
	Scan           *monitor.ManualScan  `json:"scan"` // the last "keywords" scan, if that was the last scan
	ActiveKeywords int                  `json:"active_keywords"`
	KeywordLimit   int                  `json:"keyword_limit"`
	Target         int                  `json:"target"`
	Plan           string               `json:"plan"`
	// TopUp: a Pro workspace under the target, offered "generate more".
	TopUp bool `json:"top_up"`
	// CanReview: a new review may be requested now.
	CanReview bool `json:"can_review"`
	// Allowed: the plan includes the keyword AI (Pro, or self-host).
	Allowed bool `json:"allowed"`
}

func (h *KeywordJudgeHandler) state(r *http.Request, wsID string) (keywordReviewState, error) {
	ctx := r.Context()
	st := keywordReviewState{Target: keywordjudge.TargetKeywords}
	bs, err := h.q.GetBillingState(ctx, wsID)
	if err != nil {
		return st, err
	}
	st.Plan, st.KeywordLimit = bs.Plan, int(bs.KeywordLimit)
	kws, err := h.q.ListKeywords(ctx, wsID)
	if err != nil {
		return st, err
	}
	for _, k := range kws {
		if k.IsActive {
			st.ActiveKeywords++
		}
	}
	st.Allowed = h.allowed(st.Plan)
	st.TopUp = st.Allowed && st.Plan == "pro" && st.ActiveKeywords < st.Target && st.ActiveKeywords < st.KeywordLimit
	if raw, err := h.q.GetKeywordReview(ctx, wsID); err == nil && len(raw) > 0 {
		var rev keywordjudge.Stored
		if json.Unmarshal(raw, &rev) == nil {
			st.Review = &rev
		}
	}
	scan := h.lastScan(r, wsID)
	if scan != nil && scan.Kind == monitor.ScanKeywords {
		shown := *scan
		shown.By = "" // may be an operator's email
		st.Scan = &shown
	}
	now := time.Now().UTC()
	attempt, _ := h.q.KeywordReviewAttemptAt(ctx, wsID)
	st.CanReview = st.Allowed && (scan == nil || scan.CanRequest(now)) &&
		(st.Review == nil || now.Sub(st.Review.GeneratedAt) > reviewEvery) &&
		now.Sub(attempt) > retryEvery
	return st, nil
}

// allowed: the keyword AI runs a strong model, so it is a Pro feature on
// the hosted service; self-host (credits not enforced) always has it.
func (h *KeywordJudgeHandler) allowed(plan string) bool {
	return !h.credits.Enforced() || plan == "pro"
}

func (h *KeywordJudgeHandler) lastScan(r *http.Request, wsID string) *monitor.ManualScan {
	raw, err := h.q.GetWorkspaceSettings(r.Context(), wsID)
	if err != nil {
		return nil
	}
	var s struct {
		Scan *monitor.ManualScan `json:"manual_scan"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	return s.Scan
}

// GetReview handles GET /keywords/review.
func (h *KeywordJudgeHandler) GetReview(w http.ResponseWriter, r *http.Request) {
	st, err := h.state(r, middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the keyword review")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// RequestReview handles POST /keywords/review: the judge runs on the worker
// within a minute (a manual scan of kind "keywords").
func (h *KeywordJudgeHandler) RequestReview(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	st, err := h.state(r, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the keyword review")
		return
	}
	if !st.Allowed {
		writeError(w, http.StatusForbidden, "keyword reviews are part of the Pro plan")
		return
	}
	if !st.CanReview {
		writeError(w, http.StatusTooManyRequests, "a review is already running or was made in the last 6 hours")
		return
	}
	by := "customer"
	if c := middleware.ClaimsFromContext(r.Context()); c != nil && c.Email != "" {
		by = c.Email
	}
	now := time.Now().UTC()
	if err := h.q.MarkKeywordReviewAttempt(r.Context(), wsID, now); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to request a review")
		return
	}
	scan, _ := monitor.NewManualScan(monitor.ScanKeywords, by, now)
	raw, _ := json.Marshal(scan)
	stored, err := h.q.RequestManualScan(r.Context(), wsID, raw, monitor.StaleBefore(now))
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to request a review")
	case !stored:
		writeError(w, http.StatusConflict, "a scan of this workspace is running; try again in a few minutes")
	default:
		writeJSON(w, http.StatusAccepted, scan)
	}
}

// Decide handles POST /keywords/review/decision {id, decision}: what the
// customer did with a suggestion, so the next review knows.
func (h *KeywordJudgeHandler) Decide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch body.Decision {
	case "applied", "dismissed":
	default:
		writeError(w, http.StatusBadRequest, `decision must be "applied" or "dismissed"`)
		return
	}
	if len(body.ID) == 0 || len(body.ID) > 32 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	ok, err := h.q.SetKeywordReviewDecision(r.Context(), middleware.WorkspaceID(r.Context()), body.ID, body.Decision)
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to save the decision")
	case !ok:
		writeError(w, http.StatusNotFound, "no such suggestion in the current review")
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Suggest handles POST /keywords/suggest {count}: new keywords to choose
// from (nothing is saved; the page adds the ones the customer ticks).
func (h *KeywordJudgeHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	if body.Count <= 0 {
		body.Count = 20
	}
	if body.Count > maxSuggest {
		body.Count = maxSuggest
	}
	bs, err := h.q.GetBillingState(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check your plan")
		return
	}
	if !h.allowed(bs.Plan) {
		writeError(w, http.StatusForbidden, "generating keywords is part of the Pro plan")
		return
	}
	if !h.allowSuggest(wsID, time.Now()) {
		writeError(w, http.StatusTooManyRequests, "still writing the last batch; try again in a few seconds")
		return
	}
	out, err := keywordjudge.Suggest(r.Context(), h.q, h.suggester, wsID, h.credits.PaidPlatformsLocked(r.Context(), wsID), body.Count)
	switch {
	case errors.Is(err, keywordjudge.ErrNoProfile):
		writeError(w, http.StatusConflict, "finish onboarding first: keywords are written from your product profile")
	case err != nil:
		writeError(w, http.StatusBadGateway, "could not write keywords right now; try again")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"keywords": out})
	}
}

func (h *KeywordJudgeHandler) allowSuggest(wsID string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if now.Sub(h.lastCall[wsID]) < suggestEvery {
		return false
	}
	h.lastCall[wsID] = now
	return true
}
