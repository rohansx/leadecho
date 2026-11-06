package billing

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/database"
)

// Official Standard Webhooks test vector (standardwebhooks.com spec).
const (
	vecSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	vecID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
	vecTS     = 1614265330
	vecBody   = `{"test": 2432232314}`
	vecSig    = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
)

func vecHeaders(sig string) http.Header {
	h := http.Header{}
	h.Set("webhook-id", vecID)
	h.Set("webhook-timestamp", strconv.Itoa(vecTS))
	h.Set("webhook-signature", sig)
	return h
}

func TestVerifyWebhook(t *testing.T) {
	at := time.Unix(vecTS, 0).Add(30 * time.Second)
	cases := []struct {
		name   string
		secret string
		h      http.Header
		body   string
		now    time.Time
		ok     bool
	}{
		{"valid", vecSecret, vecHeaders(vecSig), vecBody, at, true},
		{"one of several signatures", vecSecret, vecHeaders("v1,bm90LWl0 " + vecSig), vecBody, at, true},
		{"tampered body", vecSecret, vecHeaders(vecSig), `{"test": 1}`, at, false},
		{"wrong secret", "whsec_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", vecHeaders(vecSig), vecBody, at, false},
		{"stale timestamp (replay)", vecSecret, vecHeaders(vecSig), vecBody, time.Unix(vecTS, 0).Add(10 * time.Minute), false},
		{"missing headers", vecSecret, http.Header{}, vecBody, at, false},
		{"empty secret", "", vecHeaders(vecSig), vecBody, at, false},
	}
	for _, c := range cases {
		err := VerifyWebhook(c.secret, c.h, []byte(c.body), c.now)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestHandleEventDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)
	svc := NewService(q, New(q, true), Products{Starter: "pdt_starter", Pro: "pdt_pro", Topup: "pdt_topup"})
	ws := newWorkspace(t, ctx, pool, q, 300)
	cus := "cus_" + ws // unique per run: the customer fallback must find this workspace
	p1 := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	p2 := p1.AddDate(0, 1, 0)

	sub := func(typ, product string, prev, next time.Time) Event {
		e := Event{Type: typ}
		e.Data.SubscriptionID, e.Data.ProductID = "sub_1", product
		e.Data.Metadata = map[string]string{"workspace_id": ws}
		e.Data.Customer.CustomerID = cus
		e.Data.PreviousBillingDate, e.Data.NextBillingDate = &prev, &next
		return e
	}
	state := func() database.GetBillingStateRow {
		st, err := q.GetBillingState(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	// Activation and the renewal event for the same period grant once.
	for _, typ := range []string{"subscription.active", "subscription.renewed", "subscription.active"} {
		if err := svc.HandleEvent(ctx, sub(typ, "pdt_starter", p1, p2)); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}
	st := state()
	if st.Plan != "starter" || st.CreditBalance != 5300 || st.KeywordLimit != 15 || st.DodoCustomerID.String != cus || !st.PlanRenewsAt.Valid {
		t.Fatalf("after activation: %+v", st)
	}

	// Next period renews the allowance (credits roll over).
	if err := svc.HandleEvent(ctx, sub("subscription.renewed", "pdt_starter", p2, p2.AddDate(0, 1, 0))); err != nil {
		t.Fatal(err)
	}
	if st = state(); st.CreditBalance != 10300 {
		t.Fatalf("after renewal: balance %d, want 10300", st.CreditBalance)
	}

	// Upgrade mid-period grants the Pro allowance once.
	for i := 0; i < 2; i++ {
		if err := svc.HandleEvent(ctx, sub("subscription.plan_changed", "pdt_pro", p2, p2.AddDate(0, 1, 0))); err != nil {
			t.Fatal(err)
		}
	}
	if st = state(); st.Plan != "pro" || st.KeywordLimit != 50 || st.CreditBalance != 30300 {
		t.Fatalf("after upgrade: %+v", st)
	}

	// Top-up: one-time payment, found by customer id, applied once.
	pay := Event{Type: "payment.succeeded"}
	pay.Data.PaymentID = "pay_1"
	pay.Data.Customer.CustomerID = cus
	pay.Data.ProductCart = []CartItem{{ProductID: "pdt_topup", Quantity: 2}}
	for i := 0; i < 2; i++ {
		if err := svc.HandleEvent(ctx, pay); err != nil {
			t.Fatal(err)
		}
	}
	if st = state(); st.CreditBalance != 32300 {
		t.Fatalf("after top-up: balance %d, want 32300", st.CreditBalance)
	}

	// A subscription payment is not a top-up.
	subPay := Event{Type: "payment.succeeded"}
	subPay.Data.PaymentID, subPay.Data.SubscriptionID = "pay_2", "sub_1"
	subPay.Data.Customer.CustomerID = cus
	subPay.Data.ProductCart = []CartItem{{ProductID: "pdt_pro", Quantity: 1}}
	if err := svc.HandleEvent(ctx, subPay); err != nil {
		t.Fatal(err)
	}
	if st = state(); st.CreditBalance != 32300 {
		t.Fatalf("subscription payment changed balance: %d", st.CreditBalance)
	}

	// Cancellation drops to Free; purchased credits stay.
	if err := svc.HandleEvent(ctx, sub("subscription.cancelled", "pdt_pro", p2, p2)); err != nil {
		t.Fatal(err)
	}
	if st = state(); st.Plan != "free" || st.KeywordLimit != 3 || st.DodoSubscriptionID.Valid || st.CreditBalance != 32300 {
		t.Fatalf("after cancel: %+v", st)
	}

	// Unknown workspace / product: acknowledged, no change, no error.
	stray := sub("subscription.active", "pdt_unknown", p1, p2)
	if err := svc.HandleEvent(ctx, stray); err != nil {
		t.Fatalf("unknown product: %v", err)
	}
	stray = sub("subscription.active", "pdt_starter", p1, p2)
	stray.Data.Metadata = nil
	stray.Data.Customer.CustomerID = "cus_nobody"
	if err := svc.HandleEvent(ctx, stray); err != nil {
		t.Fatalf("unknown workspace: %v", err)
	}
}
