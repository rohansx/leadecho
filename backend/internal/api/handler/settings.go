package handler

import (
	"encoding/json"
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

// Supported AI providers for BYOK.
var supportedProviders = map[string]bool{
	"glm":    true,
	"openai": true,
	"voyage": true,
}

// apiKeysSettings is the structure stored in workspaces.settings["api_keys"].
type apiKeysSettings map[string]string // provider → encrypted key

// APIKeyStatus is the response for GET: shows which keys are configured (masked).
type APIKeyStatus struct {
	Provider  string `json:"provider"`
	IsSet     bool   `json:"is_set"`
	MaskedKey string `json:"masked_key,omitempty"`
}

// SettingsHandler handles workspace settings API endpoints.
type SettingsHandler struct {
	queries       *database.Queries
	encryptionKey []byte
}

func NewSettingsHandler(queries *database.Queries, encryptionKey []byte) *SettingsHandler {
	return &SettingsHandler{queries: queries, encryptionKey: encryptionKey}
}

// GetAPIKeys returns the status of configured API keys (masked, never raw).
func (h *SettingsHandler) GetAPIKeys(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	settings, err := h.loadSettings(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	keys := h.extractAPIKeys(settings)
	result := make([]APIKeyStatus, 0, len(supportedProviders))
	for provider := range supportedProviders {
		status := APIKeyStatus{Provider: provider}
		if enc, ok := keys[provider]; ok && enc != "" {
			decrypted, err := crypto.Decrypt(h.encryptionKey, enc)
			if err == nil && decrypted != "" {
				status.IsSet = true
				status.MaskedKey = crypto.MaskKey(decrypted)
			}
		}
		result = append(result, status)
	}

	_ = wsID // used via middleware.WorkspaceID
	writeJSON(w, http.StatusOK, result)
}

// SaveAPIKey saves or updates a single API key for a provider.
func (h *SettingsHandler) SaveAPIKey(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !supportedProviders[body.Provider] {
		writeError(w, http.StatusBadRequest, "unsupported provider")
		return
	}
	if body.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	settings, err := h.loadSettings(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	keys := h.extractAPIKeys(settings)

	encrypted, err := crypto.Encrypt(h.encryptionKey, body.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt key")
		return
	}
	keys[body.Provider] = encrypted

	settings["api_keys"] = keys
	if err := h.saveSettings(r, wsID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, APIKeyStatus{
		Provider:  body.Provider,
		IsSet:     true,
		MaskedKey: crypto.MaskKey(body.APIKey),
	})
}

// DeleteAPIKey removes a provider's API key.
func (h *SettingsHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !supportedProviders[body.Provider] {
		writeError(w, http.StatusBadRequest, "unsupported provider")
		return
	}

	settings, err := h.loadSettings(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	keys := h.extractAPIKeys(settings)
	delete(keys, body.Provider)

	settings["api_keys"] = keys
	if err := h.saveSettings(r, wsID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, APIKeyStatus{
		Provider: body.Provider,
		IsSet:    false,
	})
}

// loadSettings reads the workspace's settings JSONB column.
func (h *SettingsHandler) loadSettings(r *http.Request) (map[string]any, error) {
	wsID := middleware.WorkspaceID(r.Context())
	raw, err := h.queries.GetWorkspaceSettings(r.Context(), wsID)
	if err != nil {
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		settings = make(map[string]any)
	}
	return settings, nil
}

// extractAPIKeys pulls the api_keys map from settings, converting from any types.
func (h *SettingsHandler) extractAPIKeys(settings map[string]any) apiKeysSettings {
	keys := make(apiKeysSettings)
	if raw, ok := settings["api_keys"]; ok {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					keys[k] = s
				}
			}
		}
	}
	return keys
}

// saveSettings writes the settings map back to the workspace.
func (h *SettingsHandler) saveSettings(r *http.Request, wsID string, settings map[string]any) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return h.queries.UpdateWorkspaceSettings(r.Context(), database.UpdateWorkspaceSettingsParams{
		ID:       wsID,
		Settings: data,
	})
}
