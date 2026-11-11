package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

const webhookTolerance = 5 * time.Minute

// VerifyWebhook checks a Standard Webhooks signature (what Dodo sends):
// base64(HMAC-SHA256(secret, id.timestamp.body)) in "v1,<sig>" entries,
// with the secret's "whsec_" prefix stripped and base64-decoded, and a
// timestamp within 5 minutes to stop replays.
func VerifyWebhook(secret string, h http.Header, body []byte, now time.Time) error {
	id, ts, sigs := h.Get("webhook-id"), h.Get("webhook-timestamp"), h.Get("webhook-signature")
	if secret == "" || id == "" || ts == "" || sigs == "" {
		return errors.New("webhook: missing secret or signature headers")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("webhook: bad timestamp")
	}
	if d := now.Sub(time.Unix(sec, 0)); d > webhookTolerance || d < -webhookTolerance {
		return errors.New("webhook: timestamp outside tolerance")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return errors.New("webhook: bad secret")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, entry := range strings.Fields(sigs) {
		version, sig, ok := strings.Cut(entry, ",")
		if !ok || version != "v1" {
			continue
		}
		if got, err := base64.StdEncoding.DecodeString(sig); err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return errors.New("webhook: signature mismatch")
}

// Products maps Dodo product ids to what they sell.
type Products struct {
	Starter, Pro, Topup string
}

func (p Products) plan(productID string) (Plan, bool) {
	switch {
	case productID == "":
		return Plan{}, false
	case productID == p.Starter:
		return PlanByID("starter")
	case productID == p.Pro:
		return PlanByID("pro")
	}
	return Plan{}, false
}

// CartItem is one product line of a Dodo payment.
type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Event is the part of a Dodo webhook we act on.
type Event struct {
	Type string `json:"type"`
	Data struct {
		SubscriptionID      string            `json:"subscription_id"`
		PaymentID           string            `json:"payment_id"`
		ProductID           string            `json:"product_id"`
		Status              string            `json:"status"`
		Metadata            map[string]string `json:"metadata"`
		NextBillingDate     *time.Time        `json:"next_billing_date"`
		PreviousBillingDate *time.Time        `json:"previous_billing_date"`
		ProductCart         []CartItem        `json:"product_cart"`
		Customer            struct {
			CustomerID string `json:"customer_id"`
			Email      string `json:"email"`
		} `json:"customer"`
	} `json:"data"`
}

// Service applies Dodo events to workspaces.
type Service struct {
	q        *database.Queries
	credits  *Credits
	products Products
}

func NewService(q *database.Queries, credits *Credits, products Products) *Service {
	return &Service{q: q, credits: credits, products: products}
}

// HandleEvent applies one verified event. Events for unknown workspaces or
// products are acknowledged (nil) so Dodo doesn't retry them forever.
func (s *Service) HandleEvent(ctx context.Context, e Event) error {
	ws, err := s.workspaceFor(ctx, e)
	if err != nil || ws == "" {
		return err
	}
	d := e.Data
	switch e.Type {
	case "subscription.active", "subscription.renewed", "subscription.plan_changed":
		plan, ok := s.products.plan(d.ProductID)
		if !ok {
			return nil
		}
		if err := s.setPlan(ctx, ws, plan, d.Customer.CustomerID, d.SubscriptionID, d.NextBillingDate); err != nil {
			return err
		}
		// One allowance per subscription, billing period and plan: the active
		// and renewed events for the same period grant once; an upgrade grants
		// the new plan's allowance once.
		ref := fmt.Sprintf("sub:%s:%s:%s", d.SubscriptionID, periodKey(d.PreviousBillingDate, d.NextBillingDate), plan.ID)
		_, _, err := s.credits.Grant(ctx, ws, plan.MonthlyCredits, ReasonPlan, ref)
		return err
	case "subscription.cancelled", "subscription.expired", "subscription.failed":
		free, _ := PlanByID("free")
		return s.setPlan(ctx, ws, free, d.Customer.CustomerID, "", nil)
	case "payment.succeeded":
		if d.SubscriptionID != "" || d.PaymentID == "" {
			return nil // subscription charges are handled by subscription.* events
		}
		n := 0
		for _, item := range d.ProductCart {
			if item.ProductID == s.products.Topup && s.products.Topup != "" {
				n += max(item.Quantity, 1) * TopupCredits
			}
		}
		if n == 0 {
			return nil
		}
		_, _, err := s.credits.Grant(ctx, ws, n, ReasonTopup, "pay:"+d.PaymentID)
		return err
	}
	return nil // on_hold, past_due, updated, …: no change to credits
}

func (s *Service) setPlan(ctx context.Context, ws string, plan Plan, customerID, subID string, renews *time.Time) error {
	// A paid Dodo plan includes paid data (treg); dropping to Free removes it.
	source, paid := "dodo", true
	if plan.PriceCents == 0 {
		source, paid = "free", false
	}
	params := database.SetWorkspacePlanParams{
		WorkspaceID: ws, Plan: plan.ID, KeywordLimit: int32(plan.KeywordLimit), PlanSource: source, PaidData: paid,
	}
	if customerID != "" {
		params.DodoCustomerID = pgtype.Text{String: customerID, Valid: true}
	}
	if subID != "" {
		params.DodoSubscriptionID = pgtype.Text{String: subID, Valid: true}
	}
	if renews != nil {
		params.PlanRenewsAt = pgtype.Timestamptz{Time: *renews, Valid: true}
	}
	before, berr := s.q.GetBillingState(ctx, ws)
	if err := s.q.SetWorkspacePlan(ctx, params); err != nil {
		return fmt.Errorf("set plan: %w", err)
	}
	// An upgrade (not a renewal) starts the keywords the old limit left paused.
	if berr == nil && params.KeywordLimit > before.KeywordLimit {
		if _, err := s.q.ActivateKeywordsUpTo(ctx, ws, params.KeywordLimit); err != nil {
			return fmt.Errorf("activate keywords: %w", err)
		}
	}
	return nil
}

// workspaceFor: checkout metadata first, then the stored Dodo customer id.
func (s *Service) workspaceFor(ctx context.Context, e Event) (string, error) {
	if ws := e.Data.Metadata["workspace_id"]; ws != "" {
		if _, err := s.q.GetBillingState(ctx, ws); err == nil {
			return ws, nil
		} else if !errors.Is(err, pgx.ErrNoRows) && !isInvalidUUID(err) {
			return "", fmt.Errorf("lookup workspace: %w", err)
		}
	}
	if c := e.Data.Customer.CustomerID; c != "" {
		ws, err := s.q.FindWorkspaceByDodoCustomer(ctx, pgtype.Text{String: c, Valid: true})
		if err == nil {
			return ws, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup customer: %w", err)
		}
	}
	return "", nil
}

func periodKey(prev, next *time.Time) string {
	switch {
	case prev != nil && !prev.IsZero():
		return prev.UTC().Format("2006-01-02")
	case next != nil && !next.IsZero():
		return "next-" + next.UTC().Format("2006-01-02")
	}
	return "unknown"
}

func isInvalidUUID(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid input syntax for type uuid")
}
