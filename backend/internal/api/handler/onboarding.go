package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/knowledge"
	"leadecho/internal/llm"
	"leadecho/internal/onboarding"
	"leadecho/internal/reply"
)

// OnboardingHandler manages workspace onboarding state stored in settings JSONB.
type OnboardingHandler struct {
	q         *database.Queries
	scrapling *browser.ScraplingClient
	llmRouter *llm.Router
	logger    zerolog.Logger
	scanner   WorkspaceScanner

	// onboarding v2 (optional; see onboarding_flow.go)
	kb     *knowledge.Service
	finder onboarding.CompetitorFinder
	fetch  onboarding.PageFetcher // nil = SSRF-safe default
	search onboarding.Searcher    // nil = public sources
}

// WorkspaceScanner starts an immediate crawl of one workspace (monitor.Monitor).
type WorkspaceScanner interface{ ScanWorkspaceNow(workspaceID string) }

// SetScanner enables the immediate first scan after onboarding. Optional.
func (h *OnboardingHandler) SetScanner(s WorkspaceScanner) { h.scanner = s }

func NewOnboardingHandler(q *database.Queries, scrapling *browser.ScraplingClient, llmRouter *llm.Router, logger zerolog.Logger) *OnboardingHandler {
	return &OnboardingHandler{q: q, scrapling: scrapling, llmRouter: llmRouter, logger: logger}
}

type onboardingState struct {
	Completed bool `json:"completed"`
	Step      int  `json:"step"`
}

// GetOnboardingStatus returns the current onboarding state from workspace.settings.
func (h *OnboardingHandler) GetOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	raw, err := h.q.GetWorkspaceSettings(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}

	var settings map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &settings)
	}

	state := onboardingState{Completed: false, Step: 1}
	if obRaw, ok := settings["onboarding"]; ok {
		_ = json.Unmarshal(obRaw, &state)
	}

	writeJSON(w, http.StatusOK, state)
}

// UpdateOnboarding merges the supplied fields into workspace.settings.onboarding.
func (h *OnboardingHandler) UpdateOnboarding(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var patch struct {
		Completed *bool `json:"completed"`
		Step      *int  `json:"step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	raw, _ := h.q.GetWorkspaceSettings(ctx, wsID)

	var settings map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &settings)
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}

	// Read existing onboarding state
	state := onboardingState{Completed: false, Step: 1}
	if obRaw, ok := settings["onboarding"]; ok {
		_ = json.Unmarshal(obRaw, &state)
	}

	// Apply patch
	if patch.Completed != nil {
		state.Completed = *patch.Completed
	}
	if patch.Step != nil {
		state.Step = *patch.Step
	}

	obJSON, _ := json.Marshal(state)
	settings["onboarding"] = obJSON

	newRaw, _ := json.Marshal(settings)
	if err := h.q.UpdateWorkspaceSettings(ctx, database.UpdateWorkspaceSettingsParams{
		ID:       wsID,
		Settings: newRaw,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	writeJSON(w, http.StatusOK, state)
}

// AnalyzeURL scrapes a product URL and uses AI to extract product information.
// POST /settings/onboarding/analyze-url
func (h *OnboardingHandler) AnalyzeURL(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	res, err := h.runAnalysis(r.Context(), wsID, body.URL, func(string) {})
	switch {
	case errors.Is(err, errBadURL):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeJSON(w, http.StatusOK, res.Analysis)
	}
}

// completeRequest holds the user-reviewed onboarding data.
type completeRequest struct {
	ProductName string   `json:"product_name"`
	Description string   `json:"description"`
	PainPoints  []string `json:"pain_points"`
	Keywords    []string `json:"keywords"`
	Platforms   []string `json:"platforms"`
	Subreddits  []string `json:"subreddits"`

	// The rest of the analysis, previously dropped (persisted to brand_profiles).
	Competitors    []string `json:"competitors"`
	Features       []string `json:"features"`
	TargetAudience string   `json:"target_audience"`
	SourceURL      string   `json:"source_url"`

	// onboarding v2: the edited brand profile and the confirmed listeners
	// (one monitoring profile each). When Listeners is set, ProductName,
	// Keywords and Platforms are not needed.
	BrandProfile *onboarding.BrandProfile `json:"brand_profile"`
	Listeners    []listenerInput          `json:"listeners"`
}

// brandProfile returns the edited brand profile, or one built from the
// legacy flat fields.
func (b completeRequest) brandProfile() onboarding.BrandProfile {
	if b.BrandProfile != nil {
		return *b.BrandProfile
	}
	pa := &ai.ProductAnalysis{
		ProductName: b.ProductName, Description: b.Description, PainPoints: b.PainPoints,
		Competitors: b.Competitors, Features: b.Features, SuggestedKeywords: b.Keywords,
	}
	bp := onboarding.FromAnalysis(pa, b.SourceURL)
	bp.TargetAudience = b.TargetAudience
	return bp
}

// onboardingCompleted reports whether this workspace has already finished onboarding.
func (h *OnboardingHandler) onboardingCompleted(ctx context.Context, wsID string) bool {
	raw, err := h.q.GetWorkspaceSettings(ctx, wsID)
	if err != nil || len(raw) == 0 {
		return false
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	var state onboardingState
	if obRaw, ok := settings["onboarding"]; ok {
		_ = json.Unmarshal(obRaw, &state)
	}
	return state.Completed
}

// Complete creates all resources from the reviewed onboarding data and marks onboarding done.
// POST /settings/onboarding/complete
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body completeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	h.logger.Info().
		Str("workspace_id", wsID).
		Str("product_name", body.ProductName).
		Int("pain_points", len(body.PainPoints)).
		Int("keywords", len(body.Keywords)).
		Strs("platforms", body.Platforms).
		Strs("subreddits", body.Subreddits).
		Msg("onboarding complete request received")

	if body.Subreddits == nil {
		body.Subreddits = []string{}
	}
	if len(body.Listeners) > 0 {
		h.completeWithListeners(w, r, wsID, body)
		return
	}

	body.ProductName = strings.TrimSpace(body.ProductName)
	if body.ProductName == "" {
		writeError(w, http.StatusBadRequest, "product_name is required")
		return
	}

	ctx := r.Context()

	// Idempotency: if onboarding is already complete, don't create duplicate
	// monitoring profiles/keywords on a re-submit.
	if h.onboardingCompleted(ctx, wsID) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "completed", "already_completed": true})
		return
	}

	// Normalize array fields so a nil slice never hits a NOT NULL column.
	if len(body.Platforms) == 0 {
		body.Platforms = []string{"hackernews", "reddit", "twitter", "linkedin"}
	}

	// 1. Create monitoring profile with pain points
	profile, err := h.q.CreateMonitoringProfile(ctx, database.CreateMonitoringProfileParams{
		WorkspaceID: wsID,
		Name:        body.ProductName,
		Description: body.Description,
		IsActive:    true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}

	// 2. Embed and store pain points
	if len(body.PainPoints) > 0 && h.llmRouter != nil {
		if err := embedAndStore(ctx, h.q, h.llmRouter, profile.ID, wsID, body.PainPoints); err != nil {
			// Non-fatal: keywords still work, but semantic matching is off until
			// an embedding provider is configured and the profile is re-saved.
			h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: pain-point embeddings not stored")
		}
	}

	// 3. Create keywords — tolerate duplicates, but surface real failures so we
	//    never report "completed" while having silently created zero monitors.
	keywordsCreated := 0
	keywordErrors := 0
	for _, keyword := range body.Keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if _, err := h.q.CreateKeyword(ctx, database.CreateKeywordParams{
			WorkspaceID:   wsID,
			ProfileID:     profile.ID,
			Term:          keyword,
			Platforms:     body.Platforms,
			IsActive:      true,
			MatchType:     "contains",
			NegativeTerms: []string{},
			Subreddits:    body.Subreddits,
		}); err != nil {
			if isUniqueViolation(err) {
				// Duplicate term (already monitored) is benign.
				continue
			}
			// Real error — don't silently swallow; count and report.
			keywordErrors++
			continue
		}
		keywordsCreated++
	}

	// Guard: require at least one usable keyword. Without keywords the monitor has
	// nothing to crawl, so the inbox stays empty forever. We also block if every
	// keyword failed with a real error so the user can retry.
	if len(body.Keywords) > 0 && keywordsCreated == 0 {
		if keywordErrors > 0 {
			writeError(w, http.StatusInternalServerError, "failed to create keywords")
		} else {
			writeError(w, http.StatusBadRequest, "all keywords were empty or duplicates; add at least one unique keyword")
		}
		return
	}

	// 4. Persist the full analysis (competitors, features, audience) and
	//    mark onboarding complete.
	h.saveBrandProfile(ctx, wsID, body.brandProfile())
	h.markOnboardingComplete(ctx, wsID)

	// Kick off the first crawl (7-day backfill) instead of waiting for the next tick.
	if keywordsCreated > 0 {
		h.startFirstScan(wsID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "completed",
		"profile_id":       profile.ID,
		"keywords_created": keywordsCreated,
	})
}

// completeWithListeners is the v2 completion: one monitoring profile per
// confirmed listener, the edited brand profile, then the 7-day backfill.
func (h *OnboardingHandler) completeWithListeners(w http.ResponseWriter, r *http.Request, wsID string, body completeRequest) {
	ctx := r.Context()
	if len(body.Listeners) > maxListeners {
		writeError(w, http.StatusBadRequest, "too many listeners")
		return
	}
	for i := range body.Listeners {
		l, err := body.Listeners[i].validate()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		body.Listeners[i] = l
	}
	if body.BrandProfile != nil {
		bp, err := body.BrandProfile.Normalized()
		if err != nil {
			writeError(w, http.StatusBadRequest, "brand_profile: "+err.Error())
			return
		}
		body.BrandProfile = &bp
	}
	if h.onboardingCompleted(ctx, wsID) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "completed", "already_completed": true})
		return
	}
	created, profileIDs, err := h.completeListeners(ctx, wsID, body)
	if err != nil {
		h.logger.Error().Err(err).Str("workspace_id", wsID).Msg("onboarding: create listeners")
		writeError(w, http.StatusInternalServerError, "failed to create listeners")
		return
	}
	if created == 0 {
		writeError(w, http.StatusBadRequest, "all keywords were duplicates; add at least one unique keyword")
		return
	}
	if body.BrandProfile != nil {
		h.saveBrandProfile(ctx, wsID, *body.BrandProfile)
	}
	h.markOnboardingComplete(ctx, wsID)
	h.startFirstScan(wsID)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "completed", "profile_ids": profileIDs, "keywords_created": created,
		"backfill_days": int(backfillLookback.Hours() / 24),
	})
}

// saveBrandProfile is best-effort: onboarding must not fail on it.
func (h *OnboardingHandler) saveBrandProfile(ctx context.Context, wsID string, bp onboarding.BrandProfile) {
	bp, err := bp.Normalized()
	if err != nil {
		return
	}
	if _, err := h.q.UpsertBrandProfile(ctx, bp.Params(wsID)); err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: save brand profile")
	}
}

func (h *OnboardingHandler) markOnboardingComplete(ctx context.Context, wsID string) {
	raw, _ := h.q.GetWorkspaceSettings(ctx, wsID)
	var settings map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &settings)
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	obJSON, _ := json.Marshal(onboardingState{Completed: true, Step: 3})
	settings["onboarding"] = obJSON
	newRaw, _ := json.Marshal(settings)
	if err := h.q.UpdateWorkspaceSettings(ctx, database.UpdateWorkspaceSettingsParams{ID: wsID, Settings: newRaw}); err != nil {
		h.logger.Error().Err(err).Str("workspace_id", wsID).Msg("onboarding: mark complete")
	}
	// voice: give the workspace a default reply voice (no-op if one exists).
	if _, err := reply.EnsureDefaultResponseProfile(ctx, h.q, wsID); err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("onboarding: default response profile")
	}
}

// embedAndStore embeds and stores pain-point phrases for a profile.
func embedAndStore(ctx context.Context, q *database.Queries, llmRouter *llm.Router, profileID, wsID string, phrases []string) error {
	vectors, err := llmRouter.EmbedTexts(ctx, wsID, llm.TaskEmbedProfiles, phrases)
	if err != nil {
		return fmt.Errorf("embed pain points: %w", err)
	}
	for i, phrase := range phrases {
		if i >= len(vectors) {
			break
		}
		if _, err := q.CreatePainPointEmbedding(ctx, database.CreatePainPointEmbeddingParams{
			ProfileID:   profileID,
			WorkspaceID: wsID,
			Phrase:      phrase,
			Embedding:   &vectors[i],
		}); err != nil {
			return fmt.Errorf("store pain point embedding: %w", err)
		}
	}
	return nil
}
