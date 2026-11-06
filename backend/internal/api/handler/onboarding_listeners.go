package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
	"leadecho/internal/onboarding"
)

const (
	backfillLookback  = 7 * 24 * time.Hour
	maxListeners      = 8
	maxKeywordsPerLis = 10
	firstValueLimit   = 10
)

// listenerPlatforms are the platforms a listener may target.
var listenerPlatforms = map[string]bool{
	"reddit": true, "hackernews": true, "twitter": true, "linkedin": true, "devto": true,
	"lobsters": true, "indiehackers": true, "exa": true, "googlenews": true, "bluesky": true, // sources
}

var defaultListenerPlatforms = []string{"reddit", "hackernews", "devto"}

// BackfillScanner is the optional part of the scanner (monitor.Monitor) used
// for the 7-day first scan and scan progress.
type BackfillScanner interface {
	ScanWorkspaceSince(workspaceID string, lookback time.Duration)
	IsScanning(workspaceID string) bool
}

func (h *OnboardingHandler) searcher() onboarding.Searcher {
	if h.search != nil {
		return h.search
	}
	return monitor.DefaultPublicSources()
}

func validPlatforms(in []string) ([]string, error) {
	if len(in) == 0 {
		return append([]string(nil), defaultListenerPlatforms...), nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if !listenerPlatforms[p] {
			return nil, fmt.Errorf("unsupported platform %q", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// BuildListeners streams one listener proposal per goal as SSE.
// POST /onboarding/listeners/build {goals, platforms}
// Events: step, query_tested, samples, listener_proposal, done.
func (h *OnboardingHandler) BuildListeners(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body struct {
		Goals     []string `json:"goals"`
		Platforms []string `json:"platforms"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	goals, err := onboarding.ParseGoals(body.Goals)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	platforms, err := validPlatforms(body.Platforms)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.q.GetBrandProfile(r.Context(), wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "analyze your product URL first")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load brand profile")
		return
	}

	b := &onboarding.Builder{Search: h.searcher()}
	if h.llmRouter != nil {
		b.Gen = h.llmRouter
	}
	s, ok := startSSE(w)
	if !ok {
		return
	}
	b.Run(r.Context(), wsID, onboarding.FromRow(row), goals, platforms, func(e onboarding.Event) { s.send(e.Type, e.Data) })
}

// SampleFeedback stores a 👍/👎 on a listener-builder sample post.
// POST /onboarding/listeners/feedback {goal, url, text, platform, label}
func (h *OnboardingHandler) SampleFeedback(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body struct {
		Goal     string `json:"goal"`
		URL      string `json:"url"`
		Text     string `json:"text"`
		Platform string `json:"platform"`
		Label    string `json:"label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, err := onboarding.ParseGoals([]string{body.Goal}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Label != "up" && body.Label != "down" {
		writeError(w, http.StatusBadRequest, "label must be up or down")
		return
	}
	if !isHTTPURL(body.URL) {
		writeError(w, http.StatusBadRequest, "url must be an http(s) URL")
		return
	}
	if body.Platform != "" && !listenerPlatforms[body.Platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}
	row, err := h.q.UpsertSampleFeedback(r.Context(), database.UpsertSampleFeedbackParams{
		WorkspaceID: wsID, Goal: body.Goal, Platform: body.Platform, Url: body.URL,
		Text: truncateRunesStr(strings.TrimSpace(body.Text), 2000), Label: body.Label,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save feedback")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID, "goal": row.Goal, "url": row.Url, "label": row.Label})
}

// listenerInput is one confirmed listener in the complete request.
type listenerInput struct {
	Goal      string   `json:"goal"`
	Name      string   `json:"name"`
	Keywords  []string `json:"keywords"`
	LooksFor  string   `json:"looks_for"`
	Skips     string   `json:"skips"`
	Platforms []string `json:"platforms"`
}

func (l listenerInput) validate() (listenerInput, error) {
	if _, err := onboarding.ParseGoals([]string{l.Goal}); err != nil {
		return l, err
	}
	l.Name = truncateRunesStr(strings.TrimSpace(l.Name), 80)
	if l.Name == "" {
		return l, errors.New("listener name is required")
	}
	var kws []string
	for _, k := range l.Keywords {
		if k = truncateRunesStr(strings.TrimSpace(k), 100); k != "" && len(kws) < maxKeywordsPerLis {
			kws = append(kws, k)
		}
	}
	if len(kws) == 0 {
		return l, fmt.Errorf("listener %q needs at least one keyword", l.Name)
	}
	l.Keywords = kws
	l.LooksFor = truncateRunesStr(strings.TrimSpace(l.LooksFor), 2000)
	l.Skips = truncateRunesStr(strings.TrimSpace(l.Skips), 2000)
	p, err := validPlatforms(l.Platforms)
	if err != nil {
		return l, err
	}
	l.Platforms = p
	return l, nil
}

// completeListeners creates one monitoring profile (+ keywords) per confirmed
// listener, links sample feedback, and returns the number of keywords created.
func (h *OnboardingHandler) completeListeners(ctx context.Context, wsID string, body completeRequest) (int, []string, error) {
	var painPoints []string
	if body.BrandProfile != nil {
		for _, p := range body.BrandProfile.PainPoints {
			painPoints = append(painPoints, p.Problem)
		}
	}
	created := 0
	var profileIDs []string
	// A retry after a partial failure reuses the listeners the earlier attempt
	// left behind instead of duplicating them: same name, same (or not yet
	// set) goal, created in the last hour. A user's own profiles are left alone.
	// ponytail: heuristic, not a transaction; wrap in a tx if the handler gets a pool.
	list, err := h.q.ListMonitoringProfiles(ctx, wsID)
	if err != nil {
		return 0, nil, fmt.Errorf("list profiles: %w", err)
	}
	for i, raw := range body.Listeners {
		l, _ := raw.validate() // validated up front in Complete
		profile, ok := retryableProfile(list, l.Name, l.Goal, time.Now())
		if !ok {
			var err error
			profile, err = h.q.CreateMonitoringProfile(ctx, database.CreateMonitoringProfileParams{
				WorkspaceID: wsID, Name: l.Name, Description: l.LooksFor, IsActive: true,
			})
			if err != nil {
				return created, profileIDs, fmt.Errorf("create profile: %w", err)
			}
		}
		profileIDs = append(profileIDs, profile.ID)
		if err := h.q.SetMonitoringProfileIntent(ctx, database.SetMonitoringProfileIntentParams{
			ID: profile.ID, WorkspaceID: wsID, LooksFor: l.LooksFor, Skips: l.Skips, Goal: l.Goal,
		}); err != nil {
			return created, profileIDs, fmt.Errorf("set listener intent: %w", err)
		}
		// Pain points drive semantic matching; embed them once (first listener)
		// so every mention is not matched N times.
		if i == 0 && len(painPoints) > 0 && h.llmRouter != nil {
			if err := embedAndStore(ctx, h.q, h.llmRouter, profile.ID, wsID, painPoints); err != nil {
				h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: pain-point embeddings not stored")
			}
		}
		pid := pgtype.UUID{}
		_ = pid.Scan(profile.ID)
		if err := h.q.LinkSampleFeedbackToProfile(ctx, database.LinkSampleFeedbackToProfileParams{ProfileID: pid, WorkspaceID: wsID, Goal: l.Goal}); err != nil {
			h.logger.Warn().Err(err).Msg("onboarding: link sample feedback")
		}
		for _, term := range l.Keywords {
			if _, err := h.q.CreateKeyword(ctx, database.CreateKeywordParams{
				WorkspaceID: wsID, ProfileID: profile.ID, Term: term, Platforms: l.Platforms,
				IsActive: true, MatchType: "contains", NegativeTerms: []string{}, Subreddits: body.Subreddits,
			}); err != nil {
				if isUniqueViolation(err) {
					continue
				}
				return created, profileIDs, fmt.Errorf("create keyword: %w", err)
			}
			created++
		}
	}
	return created, profileIDs, nil
}

// retryableProfile finds a profile a failed onboarding attempt created.
func retryableProfile(list []database.MonitoringProfile, name, goal string, now time.Time) (database.MonitoringProfile, bool) {
	for _, p := range list {
		if p.Name == name && (p.Goal == goal || p.Goal == "") && now.Sub(p.CreatedAt) < time.Hour {
			return p, true
		}
	}
	return database.MonitoringProfile{}, false
}

// startFirstScan triggers the 7-day backfill (or a plain scan if the scanner
// cannot backfill). Split deployments have no scanner: the worker tick picks
// the new keywords up without a backfill.
func (h *OnboardingHandler) startFirstScan(wsID string) {
	if h.scanner == nil {
		return
	}
	if bs, ok := h.scanner.(BackfillScanner); ok {
		bs.ScanWorkspaceSince(wsID, backfillLookback)
		return
	}
	h.scanner.ScanWorkspaceNow(wsID)
}

type firstValueResponse struct {
	Tier     string            `json:"tier"`     // leads_ready | worth_watching | recent | none
	Loosened bool              `json:"loosened"` // true when showing below the high-fit bar
	Mentions []MentionResponse `json:"mentions"`
	Counts   map[string]int    `json:"counts"`
	Scanning bool              `json:"scanning"`
}

// FirstValue returns the best mentions available right now, loosening the
// bar rather than ever returning an empty inbox while data exists.
// GET /onboarding/first-value
func (h *OnboardingHandler) FirstValue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	resp := firstValueResponse{Tier: "none", Mentions: []MentionResponse{}, Counts: map[string]int{}}
	if bs, ok := h.scanner.(BackfillScanner); ok {
		resp.Scanning = bs.IsScanning(wsID)
	}
	counts, err := h.q.CountMentionsByTier(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count mentions")
		return
	}
	for _, c := range counts {
		resp.Counts[fmt.Sprint(c.Tier)] = int(c.Count)
	}
	tiers := []struct {
		name string
		list func() ([]database.Mention, error)
	}{
		{"leads_ready", func() ([]database.Mention, error) {
			return h.q.ListMentionsLeadsReady(ctx, database.ListMentionsLeadsReadyParams{WorkspaceID: wsID, Lim: firstValueLimit})
		}},
		{"worth_watching", func() ([]database.Mention, error) {
			return h.q.ListMentionsWorthWatching(ctx, database.ListMentionsWorthWatchingParams{WorkspaceID: wsID, Lim: firstValueLimit})
		}},
		{"recent", func() ([]database.Mention, error) {
			return h.q.ListMentionsFiltered(ctx, database.ListMentionsFilteredParams{WorkspaceID: wsID, Lim: firstValueLimit})
		}},
	}
	for i, t := range tiers {
		ms, err := t.list()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load mentions")
			return
		}
		if len(ms) == 0 {
			continue
		}
		resp.Tier, resp.Loosened = t.name, i > 0
		for _, m := range ms {
			resp.Mentions = append(resp.Mentions, mentionToResponse(m))
		}
		break
	}
	writeJSON(w, http.StatusOK, resp)
}

func truncateRunesStr(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
