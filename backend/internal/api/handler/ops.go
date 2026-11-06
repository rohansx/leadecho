package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/billing"
	"leadecho/internal/database"
)

// TregBalance reads the operator's prepaid treg balance (micro-USD).
type TregBalance interface {
	Balance(ctx context.Context) (int64, error)
}

// OpsHandler is the operator console (/ops): every workspace's plan, credits
// and spend, and the switches for who may use the operator's paid data.
// Access is an email allowlist from the server environment (OPS_ADMIN_EMAILS),
// never a role a user can grant.
type OpsHandler struct {
	q       *database.Queries
	credits *billing.Credits
	treg    TregBalance // nil: treg not configured
	admins  map[string]bool
	logger  zerolog.Logger
}

func NewOpsHandler(q *database.Queries, credits *billing.Credits, treg TregBalance, admins map[string]bool, logger zerolog.Logger) *OpsHandler {
	return &OpsHandler{q: q, credits: credits, treg: treg, admins: admins, logger: logger}
}

// ParseOpsAdmins parses a comma-separated email list (lower-cased).
func ParseOpsAdmins(s string) map[string]bool {
	out := map[string]bool{}
	for _, e := range strings.Split(s, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out[e] = true
		}
	}
	return out
}

// RequireOps allows only allowlisted, signed-in emails.
func (h *OpsHandler) RequireOps(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := middleware.ClaimsFromContext(r.Context())
		if c == nil || !h.admins[strings.ToLower(strings.TrimSpace(c.Email))] {
			writeError(w, http.StatusForbidden, "not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Me is a cheap check the dashboard uses to show the /ops link.
func (h *OpsHandler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ops": true})
}

// Overview: treg balance, spend today and over 30 days, global switches.
func (h *OpsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	today, err := h.q.OpsSpendSince(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load spend")
		return
	}
	month, _ := h.q.OpsSpendSince(ctx, now.AddDate(0, 0, -30))
	resp := map[string]any{
		"spend_today":      spendJSON(today),
		"spend_30d":        spendJSON(month),
		"paid_data_paused": h.paidDataPaused(ctx),
		"credits_enforced": h.credits.Enforced(),
	}
	if h.treg != nil {
		if micro, err := h.treg.Balance(ctx); err == nil {
			resp["treg_balance_usd"] = float64(micro) / 1e6
		} else {
			resp["treg_balance_error"] = "could not read treg balance"
			h.logger.Warn().Err(err).Msg("ops: treg balance")
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func spendJSON(s database.OpsSpendSinceRow) map[string]any {
	return map[string]any{"treg_usd": float64(s.TregMicro) / 1e6, "treg_calls": s.TregCalls, "ai_usd": s.AiUsd}
}

func (h *OpsHandler) paidDataPaused(ctx context.Context) bool {
	v, err := h.q.GetOpsSetting(ctx, "paid_data_paused")
	return err == nil && string(v) == "true"
}

// Workspaces lists every workspace with plan, credits and 30-day spend.
func (h *OpsHandler) Workspaces(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.OpsListWorkspaces(r.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: list workspaces")
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, ws := range rows {
		var renews *time.Time
		if ws.PlanRenewsAt.Valid {
			renews = &ws.PlanRenewsAt.Time
		}
		out = append(out, map[string]any{
			"id": ws.ID, "name": ws.Name, "owner_email": ws.OwnerEmail, "created_at": ws.CreatedAt,
			"plan": ws.Plan, "plan_source": ws.PlanSource, "credit_balance": ws.CreditBalance,
			"keyword_limit": ws.KeywordLimit, "active_keywords": ws.ActiveKeywords,
			"paid_data": ws.PaidData, "scanning_paused": ws.ScanningPaused,
			"dodo_subscription": ws.DodoSubscriptionID.Valid, "plan_renews_at": renews,
			"last_post_at": ws.LastPostAt, "credits_used_30d": ws.CreditsUsed30d,
			"treg_usd_30d": float64(ws.TregMicro30d) / 1e6, "ai_usd_30d": ws.AiUsd30d,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Audit lists recent /ops actions.
func (h *OpsHandler) Audit(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListOpsAudit(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, map[string]any{
			"actor_email": a.ActorEmail, "action": a.Action, "workspace_name": a.WorkspaceName,
			"detail": json.RawMessage(a.Detail), "created_at": a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// SetPlan gives a workspace a plan for free ("comp"); Free removes paid data.
func (h *OpsHandler) SetPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan string `json:"plan"`
	}
	ws, ok := h.target(w, r, &body)
	if !ok {
		return
	}
	plan, found := billing.PlanByID(body.Plan)
	if !found {
		writeError(w, http.StatusBadRequest, `plan must be "free", "starter" or "pro"`)
		return
	}
	source, paid := "comp", true
	if plan.PriceCents == 0 {
		source, paid = "free", false
	}
	err := h.q.OpsSetPlan(r.Context(), database.OpsSetPlanParams{WorkspaceID: ws, Plan: plan.ID, PlanSource: source, KeywordLimit: int32(plan.KeywordLimit)})
	if err == nil {
		err = h.q.OpsSetPaidData(r.Context(), database.OpsSetPaidDataParams{WorkspaceID: ws, PaidData: paid})
	}
	h.finish(w, r, ws, "set_plan", map[string]any{"plan": plan.ID, "source": source, "paid_data": paid}, err)
}

// AdjustCredits adds (or with a negative delta, removes) credits, with a note.
func (h *OpsHandler) AdjustCredits(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Delta int    `json:"delta"`
		Note  string `json:"note"`
	}
	ws, ok := h.target(w, r, &body)
	if !ok {
		return
	}
	if body.Delta == 0 || body.Delta > 1_000_000 || body.Delta < -1_000_000 {
		writeError(w, http.StatusBadRequest, "delta must be a non-zero number of credits (max 1,000,000)")
		return
	}
	if body.Delta < 0 { // never below zero: remove at most the current balance
		bal, err := h.credits.Balance(r.Context(), ws)
		if err != nil {
			h.finish(w, r, ws, "", nil, err)
			return
		}
		body.Delta = -min(-body.Delta, bal)
	}
	var err error
	if body.Delta != 0 {
		_, _, err = h.credits.Grant(r.Context(), ws, body.Delta, billing.ReasonAdjust, "ops:"+uuid.NewString())
	}
	h.finish(w, r, ws, "adjust_credits", map[string]any{"delta": body.Delta, "note": truncateRunesStr(body.Note, 200)}, err)
}

// SetPaidData lets (or stops) a workspace spending the operator's treg balance.
func (h *OpsHandler) SetPaidData(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	ws, ok := h.target(w, r, &body)
	if !ok {
		return
	}
	err := h.q.OpsSetPaidData(r.Context(), database.OpsSetPaidDataParams{WorkspaceID: ws, PaidData: body.Enabled})
	h.finish(w, r, ws, "set_paid_data", map[string]any{"enabled": body.Enabled}, err)
}

// SetScanning pauses or resumes all scanning for a workspace.
func (h *OpsHandler) SetScanning(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paused bool `json:"paused"`
	}
	ws, ok := h.target(w, r, &body)
	if !ok {
		return
	}
	err := h.q.OpsSetScanningPaused(r.Context(), database.OpsSetScanningPausedParams{WorkspaceID: ws, ScanningPaused: body.Paused})
	h.finish(w, r, ws, "set_scanning", map[string]any{"paused": body.Paused}, err)
}

// SetPaidDataPaused is the global kill switch for all paid data (treg).
func (h *OpsHandler) SetPaidDataPaused(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	v := []byte("false")
	if body.Paused {
		v = []byte("true")
	}
	err := h.q.SetOpsSetting(r.Context(), database.SetOpsSettingParams{Key: "paid_data_paused", Value: v})
	h.finish(w, r, "", "set_paid_data_paused", map[string]any{"paused": body.Paused}, err)
}

// target decodes the body and resolves {id} to an existing workspace.
func (h *OpsHandler) target(w http.ResponseWriter, r *http.Request, body any) (string, bool) {
	ws := chi.URLParam(r, "id")
	if _, err := uuid.Parse(ws); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return "", false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return "", false
	}
	if _, err := h.q.GetBillingState(r.Context(), ws); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "workspace not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to load workspace")
		}
		return "", false
	}
	return ws, true
}

// finish audits a successful change and answers with the workspace's new state.
func (h *OpsHandler) finish(w http.ResponseWriter, r *http.Request, ws, action string, detail map[string]any, err error) {
	if err != nil {
		h.logger.Error().Err(err).Str("workspace_id", ws).Str("action", action).Msg("ops: action failed")
		writeError(w, http.StatusInternalServerError, "action failed")
		return
	}
	actor := ""
	if c := middleware.ClaimsFromContext(r.Context()); c != nil {
		actor = c.Email
	}
	raw, _ := json.Marshal(detail)
	params := database.InsertOpsAuditParams{ActorEmail: actor, Action: action, Detail: raw}
	if ws != "" {
		_ = params.WorkspaceID.Scan(ws)
	} else {
		params.WorkspaceID = pgtype.UUID{}
	}
	if aerr := h.q.InsertOpsAudit(r.Context(), params); aerr != nil {
		h.logger.Error().Err(aerr).Msg("ops: audit write failed")
	}
	h.logger.Info().Str("actor", actor).Str("action", action).Str("workspace_id", ws).Msg(fmt.Sprintf("ops: %s", action))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
