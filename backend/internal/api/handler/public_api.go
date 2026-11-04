package handler

// mcp: public REST API (/api/public/v1), authenticated by workspace API key.
// Each endpoint is a thin GET wrapper over the same tool the MCP server exposes.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/tools"
)

type PublicAPIHandler struct {
	tools   *tools.Registry
	version string
	auth    func(ctx context.Context) (string, []string)
}

func NewPublicAPIHandler(reg *tools.Registry, version string) *PublicAPIHandler {
	return &PublicAPIHandler{tools: reg, version: version, auth: middleware.APIKeyPrincipal}
}

// Probe is the unauthenticated health/identity endpoint used by vendor
// directories (treg) to verify the listing.
func (h *PublicAPIHandler) Probe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "leadecho", "version": h.version, "status": "ok",
		"mcp": "/mcp", "description": "Buying-intent social mentions, KB-grounded replies and revenue attribution",
	})
}

func (h *PublicAPIHandler) Mentions(w http.ResponseWriter, r *http.Request) {
	h.run(w, r, "search_mentions", map[string]string{"q": "query", "status": "status", "intent": "intent", "platform": "platform", "tier": "tier"})
}

func (h *PublicAPIHandler) Leads(w http.ResponseWriter, r *http.Request) {
	h.run(w, r, "list_leads", map[string]string{"stage": "stage"})
}

// run maps query params (strings plus limit/offset) to tool args and calls it.
func (h *PublicAPIHandler) run(w http.ResponseWriter, r *http.Request, tool string, strParams map[string]string) {
	qs := r.URL.Query()
	args := map[string]any{}
	for param, arg := range strParams {
		if v := qs.Get(param); v != "" {
			args[arg] = v
		}
	}
	for _, p := range []string{"limit", "offset"} {
		if v := qs.Get(p); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, p+" must be an integer")
				return
			}
			args[p] = n
		}
	}
	raw, _ := json.Marshal(args)
	ws, scopes := h.auth(r.Context())
	out, err := h.tools.Call(r.Context(), ws, scopes, tool, raw)
	var ue *tools.Error
	switch {
	case errors.As(err, &ue):
		writeError(w, http.StatusBadRequest, ue.Msg)
	case err != nil:
		zerolog.Ctx(r.Context()).Error().Err(err).Str("tool", tool).Str("workspace_id", ws).Msg("public api tool failed")
		writeError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, out)
	}
}
