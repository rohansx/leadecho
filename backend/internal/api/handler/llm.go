package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/llm"
)

type LLMHandler struct {
	router *llm.Router
}

func NewLLMHandler(router *llm.Router) *LLMHandler {
	return &LLMHandler{router: router}
}

func (h *LLMHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	cfg, err := h.router.PublicConfig(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load LLM config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *LLMHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	var body llm.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg, err := h.router.SaveConfig(r.Context(), wsID, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save LLM config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *LLMHandler) SaveProviderKey(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	provider := chi.URLParam(r, "provider")
	var body struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	status, err := h.router.SaveProviderKey(r.Context(), wsID, provider, body.APIKey, body.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *LLMHandler) DeleteProviderKey(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	provider := chi.URLParam(r, "provider")
	status, err := h.router.DeleteProviderKey(r.Context(), wsID, provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete provider key")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *LLMHandler) VerifyProvider(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	provider := chi.URLParam(r, "provider")
	if err := h.router.VerifyProvider(r.Context(), wsID, provider); err != nil {
		writeError(w, http.StatusBadGateway, "provider verification failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *LLMHandler) Usage(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	rows, err := h.router.UsageSummary(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load LLM usage")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
