package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
)

// OnboardingHandler manages workspace onboarding state stored in settings JSONB.
type OnboardingHandler struct {
	q          *database.Queries
	scrapling  *browser.ScraplingClient
	glmAPIKey  string
	openAIKey  string
	embedder   *embedding.Client
}

func NewOnboardingHandler(q *database.Queries, scrapling *browser.ScraplingClient, glmAPIKey, openAIKey string, embedder *embedding.Client) *OnboardingHandler {
	return &OnboardingHandler{q: q, scrapling: scrapling, glmAPIKey: glmAPIKey, openAIKey: openAIKey, embedder: embedder}
}

func (h *OnboardingHandler) getProvider() *ai.Provider {
	if h.glmAPIKey != "" {
		p := ai.DefaultProvider("glm", h.glmAPIKey)
		return &p
	}
	if h.openAIKey != "" {
		p := ai.DefaultProvider("openai", h.openAIKey)
		return &p
	}
	return nil
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

	// Ensure URL has scheme
	url := body.URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	provider := h.getProvider()
	if provider == nil {
		writeError(w, http.StatusBadRequest, "no AI provider configured — set GLM_API_KEY or OPENAI_API_KEY")
		return
	}

	ctx := r.Context()

	// Create analysis record
	analysis, err := h.q.CreateOnboardingAnalysis(ctx, database.CreateOnboardingAnalysisParams{
		WorkspaceID: wsID,
		SourceUrl:   url,
		RawText:     pgtype.Text{},
		Analysis:    []byte("{}"),
		Status:      "pending",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create analysis record")
		return
	}

	// Scrape the URL — try Scrapling first, fall back to HTTP
	var pageText string
	if h.scrapling != nil {
		if err := h.scrapling.Navigate(ctx, url); err == nil {
			pageText, _ = h.scrapling.GetText(ctx)
		}
	}
	if pageText == "" {
		pageText, err = browser.FetchPageText(ctx, url)
		if err != nil {
			h.q.UpdateOnboardingAnalysis(ctx, database.UpdateOnboardingAnalysisParams{
				ID:           analysis.ID,
				Analysis:     []byte("{}"),
				Status:       "failed",
				ErrorMessage: pgtype.Text{String: "failed to fetch URL: " + err.Error(), Valid: true},
			})
			writeError(w, http.StatusBadGateway, "failed to fetch URL: "+err.Error())
			return
		}
	}

	// Analyze with AI
	result, err := ai.AnalyzeProductPage(ctx, *provider, pageText)
	if err != nil {
		h.q.UpdateOnboardingAnalysis(ctx, database.UpdateOnboardingAnalysisParams{
			ID:           analysis.ID,
			Analysis:     []byte("{}"),
			Status:       "failed",
			ErrorMessage: pgtype.Text{String: "AI analysis failed: " + err.Error(), Valid: true},
		})
		writeError(w, http.StatusInternalServerError, "AI analysis failed: "+err.Error())
		return
	}

	// Save the result
	resultJSON, _ := json.Marshal(result)
	h.q.UpdateOnboardingAnalysis(ctx, database.UpdateOnboardingAnalysisParams{
		ID:           analysis.ID,
		Analysis:     resultJSON,
		Status:       "completed",
		ErrorMessage: pgtype.Text{},
	})

	writeJSON(w, http.StatusOK, result)
}

// completeRequest holds the user-reviewed onboarding data.
type completeRequest struct {
	ProductName string   `json:"product_name"`
	Description string   `json:"description"`
	PainPoints  []string `json:"pain_points"`
	Keywords    []string `json:"keywords"`
	Platforms   []string `json:"platforms"`
	Subreddits  []string `json:"subreddits"`
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
	if body.ProductName == "" {
		writeError(w, http.StatusBadRequest, "product_name is required")
		return
	}

	ctx := r.Context()

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
	if len(body.PainPoints) > 0 && h.embedder != nil {
		embedAndStore(ctx, h.q, h.embedder, profile.ID, wsID, body.PainPoints)
	}

	// 3. Create keywords
	for _, keyword := range body.Keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		h.q.CreateKeyword(ctx, database.CreateKeywordParams{
			WorkspaceID:   wsID,
			Term:          keyword,
			Platforms:     body.Platforms,
			IsActive:      true,
			MatchType:     "contains",
			NegativeTerms: []string{},
			Subreddits:    body.Subreddits,
		})
	}

	// 4. Mark onboarding complete
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
	h.q.UpdateWorkspaceSettings(ctx, database.UpdateWorkspaceSettingsParams{
		ID:       wsID,
		Settings: newRaw,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "completed",
		"profile_id": profile.ID,
	})
}

// embedAndStore embeds and stores pain-point phrases for a profile.
func embedAndStore(ctx context.Context, q *database.Queries, embedder *embedding.Client, profileID, wsID string, phrases []string) {
	vectors, err := embedder.EmbedTexts(ctx, phrases)
	if err != nil {
		return
	}
	for i, phrase := range phrases {
		if i >= len(vectors) {
			break
		}
		q.CreatePainPointEmbedding(ctx, database.CreatePainPointEmbeddingParams{
			ProfileID:   profileID,
			WorkspaceID: wsID,
			Phrase:      phrase,
			Embedding:   &vectors[i],
		})
	}
}
