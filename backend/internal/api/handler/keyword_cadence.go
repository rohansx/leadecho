package handler

// sources: per-platform keyword scan cadence and monthly data-cost preview.

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/cadence"
	"leadecho/internal/database"
)

type KeywordCadenceHandler struct{ q database.Querier }

func NewKeywordCadenceHandler(q database.Querier) *KeywordCadenceHandler {
	return &KeywordCadenceHandler{q: q}
}

// Update handles PUT /keywords/{id}/cadence with {"cadence":{"twitter":"1h"}}.
func (h *KeywordCadenceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid keyword id")
		return
	}
	var req struct {
		Cadence json.RawMessage `json:"cadence"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := cadence.Parse(req.Cadence)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(c) // re-encode: never store client bytes verbatim
	row, err := h.q.UpdateKeywordCadence(r.Context(), database.UpdateKeywordCadenceParams{
		Cadence: raw, ID: id, WorkspaceID: middleware.WorkspaceID(r.Context()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "keyword not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cadence")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID, "cadence": c})
}

type keywordCost struct {
	KeywordID string      `json:"keyword_id"`
	Term      string      `json:"term"`
	IsActive  bool        `json:"is_active"`
	Cadence   cadence.Map `json:"cadence"`
	cadence.KeywordCost
}

type costPreviewResponse struct {
	Keywords    []keywordCost      `json:"keywords"`
	USDPerMonth float64            `json:"usd_per_month"` // active keywords only
	Rates       map[string]float64 `json:"rates"`         // USD per call
	Options     []string           `json:"options"`
	Default     string             `json:"default"`
}

// CostPreview handles GET /keywords/cost-preview.
func (h *KeywordCadenceHandler) CostPreview(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListKeywordCadences(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load keywords")
		return
	}
	resp := costPreviewResponse{
		Keywords: make([]keywordCost, 0, len(rows)),
		Rates:    cadence.CostPerCall,
		Options:  []string{"15m", "30m", "1h", "3h", "6h", "12h", "24h"},
		Default:  "1h",
	}
	for _, k := range rows {
		c, err := cadence.Parse(k.Cadence)
		if err != nil {
			c = cadence.Map{}
		}
		kc := keywordCost{KeywordID: k.ID, Term: k.Term, IsActive: k.IsActive, Cadence: c, KeywordCost: cadence.Preview(k.Platforms, c)}
		if k.IsActive {
			resp.USDPerMonth += kc.USDPerMonth
		}
		resp.Keywords = append(resp.Keywords, kc)
	}
	writeJSON(w, http.StatusOK, resp)
}
