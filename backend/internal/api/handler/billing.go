package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/billing"
	"leadecho/internal/database"
)

type billingEvents interface {
	HandleEvent(ctx context.Context, e billing.Event) error
}

// BillingHandler: plan + credit balance, Dodo checkout/portal, Dodo webhooks.
type BillingHandler struct {
	q             *database.Queries
	credits       *billing.Credits
	dodo          *billing.Dodo // nil: billing not configured
	events        billingEvents
	webhookSecret string
	logger        zerolog.Logger
}

func NewBillingHandler(q *database.Queries, credits *billing.Credits, dodo *billing.Dodo, events billingEvents, webhookSecret string, logger zerolog.Logger) *BillingHandler {
	return &BillingHandler{q: q, credits: credits, dodo: dodo, events: events, webhookSecret: webhookSecret, logger: logger}
}

// Get returns the workspace's plan, balance, limits, 30-day usage and the catalog.
func (h *BillingHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	st, err := h.q.GetBillingState(ctx, wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	used, _ := h.q.CountWorkspaceKeywords(ctx, wsID)
	usage, _ := h.q.CreditUsageSince(ctx, database.CreditUsageSinceParams{WorkspaceID: wsID, Since: pgtype.Date{Time: time.Now().AddDate(0, 0, -30), Valid: true}})
	if usage == nil {
		usage = []database.CreditUsageSinceRow{}
	}
	var renews *time.Time
	if st.PlanRenewsAt.Valid {
		renews = &st.PlanRenewsAt.Time
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":           st.Plan,
		"credit_balance": st.CreditBalance,
		"keyword_limit":  st.KeywordLimit,
		"keywords_used":  used,
		"plan_renews_at": renews,
		"has_portal":     st.DodoCustomerID.Valid && h.dodo != nil,
		"usage_30d":      usage,
		"plans":          billing.Plans,
		"topup":          map[string]int{"credits": billing.TopupCredits, "price_cents": billing.TopupPriceCents},
		"costs": map[string]int{
			"post": billing.CostPost, "search": billing.CostSearch, "draft": billing.CostDraft, "chat": billing.CostChat,
		},
		"enforced": h.credits.Enforced(),
		// X, Reddit and LinkedIn need a paid plan; the app shows them locked.
		"paid_platforms_locked": h.credits.Enforced() && !st.PaidData,
		"can_checkout":          h.dodo != nil,
	})
}

// Checkout starts a Dodo hosted checkout for a plan or a top-up. Admins only.
func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	if h.dodo == nil {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	c := middleware.ClaimsFromContext(r.Context())
	if c == nil || c.Role != "admin" {
		writeError(w, http.StatusForbidden, "only workspace admins can change billing")
		return
	}
	var body struct {
		Item string `json:"item"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil ||
		(body.Item != "starter" && body.Item != "pro" && body.Item != "topup") {
		writeError(w, http.StatusBadRequest, `item must be "starter", "pro" or "topup"`)
		return
	}
	customerID := ""
	if h.q != nil {
		if st, err := h.q.GetBillingState(r.Context(), c.WorkspaceID); err == nil && st.DodoCustomerID.Valid {
			customerID = st.DodoCustomerID.String
		}
	}
	url, err := h.dodo.Checkout(r.Context(), billing.CheckoutRequest{
		Item: body.Item, WorkspaceID: c.WorkspaceID, CustomerID: customerID, Email: c.Email, Name: c.Name,
	})
	if err != nil {
		h.logger.Error().Err(err).Str("workspace_id", c.WorkspaceID).Msg("billing: checkout failed")
		writeError(w, http.StatusBadGateway, "could not start checkout, please try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"checkout_url": url})
}

// Portal returns a Dodo customer-portal link (manage plan, card, invoices).
func (h *BillingHandler) Portal(w http.ResponseWriter, r *http.Request) {
	if h.dodo == nil {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	c := middleware.ClaimsFromContext(r.Context())
	if c == nil || c.Role != "admin" {
		writeError(w, http.StatusForbidden, "only workspace admins can change billing")
		return
	}
	st, err := h.q.GetBillingState(r.Context(), c.WorkspaceID)
	if err != nil || !st.DodoCustomerID.Valid {
		writeError(w, http.StatusNotFound, "no billing account yet: choose a plan first")
		return
	}
	link, err := h.dodo.PortalLink(r.Context(), st.DodoCustomerID.String)
	if err != nil {
		h.logger.Error().Err(err).Str("workspace_id", c.WorkspaceID).Msg("billing: portal link failed")
		writeError(w, http.StatusBadGateway, "could not open the billing portal, please try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": link})
}

// Webhook receives Dodo events. Public route: trust comes only from the signature.
func (h *BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	if h.events == nil || h.webhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unreadable body")
		return
	}
	if err := billing.VerifyWebhook(h.webhookSecret, r.Header, body, time.Now()); err != nil {
		h.logger.Warn().Err(err).Msg("billing: rejected webhook")
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	var ev billing.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "invalid event")
		return
	}
	if err := h.events.HandleEvent(r.Context(), ev); err != nil {
		h.logger.Error().Err(err).Str("type", ev.Type).Msg("billing: webhook handling failed")
		writeError(w, http.StatusInternalServerError, "failed to apply event") // Dodo retries
		return
	}
	h.logger.Info().Str("type", ev.Type).Str("id", r.Header.Get("webhook-id")).Msg("billing: webhook applied")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writeOutOfCredits is the shared 402 for metered endpoints.
func writeOutOfCredits(w http.ResponseWriter, err error) bool {
	if errors.Is(err, billing.ErrNoCredits) {
		writeError(w, http.StatusPaymentRequired, billing.ErrNoCredits.Error())
		return true
	}
	return false
}
