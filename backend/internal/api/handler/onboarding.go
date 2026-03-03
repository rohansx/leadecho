package handler

import (
	"encoding/json"
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// OnboardingHandler manages workspace onboarding state stored in settings JSONB.
type OnboardingHandler struct {
	q *database.Queries
}

func NewOnboardingHandler(q *database.Queries) *OnboardingHandler {
	return &OnboardingHandler{q: q}
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
