package handler

import (
	"context"
	"net/http"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// tregSpendQuerier is the one query this handler needs (fakeable in tests).
type tregSpendQuerier interface {
	TregSpendByTool(ctx context.Context, arg database.TregSpendByToolParams) ([]database.TregSpendByToolRow, error)
}

// DataSpendHandler reports a workspace's treg (paid data) spend this month,
// from our treg_calls ledger. It is a preview figure, not an invoice: invoices
// come from treg's usage/by-tag ledger.
type DataSpendHandler struct {
	q   tregSpendQuerier
	now func() time.Time
}

func NewDataSpendHandler(q *database.Queries) *DataSpendHandler {
	return &DataSpendHandler{q: q, now: time.Now}
}

type DataSpendResponse struct {
	PeriodStart    string                        `json:"period_start"`
	TotalCostMicro int64                         `json:"total_cost_micro"`
	TotalCostUSD   float64                       `json:"total_cost_usd"`
	Calls          int64                         `json:"calls"`
	ByTool         []database.TregSpendByToolRow `json:"by_tool"`
}

func monthStartUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Get handles GET /api/v1/usage/data-spend.
func (h *DataSpendHandler) Get(w http.ResponseWriter, r *http.Request) {
	since := monthStartUTC(h.now())
	rows, err := h.q.TregSpendByTool(r.Context(), database.TregSpendByToolParams{
		WorkspaceID: middleware.WorkspaceID(r.Context()),
		Since:       since,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load data spend")
		return
	}
	resp := DataSpendResponse{PeriodStart: since.Format(time.RFC3339), ByTool: rows}
	if resp.ByTool == nil {
		resp.ByTool = []database.TregSpendByToolRow{}
	}
	for _, row := range rows {
		resp.TotalCostMicro += row.CostMicro
		resp.Calls += row.Calls
	}
	resp.TotalCostUSD = float64(resp.TotalCostMicro) / 1e6
	writeJSON(w, http.StatusOK, resp)
}
