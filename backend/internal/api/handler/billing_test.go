package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/billing"
)

type fakeEvents struct {
	got []billing.Event
	err error
}

func (f *fakeEvents) HandleEvent(_ context.Context, e billing.Event) error {
	f.got = append(f.got, e)
	return f.err
}

const testWebhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func signedWebhook(t *testing.T, body string, at time.Time) *http.Request {
	t.Helper()
	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(testWebhookSecret, "whsec_"))
	id, ts := "msg_1", strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + body))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/dodo", strings.NewReader(body))
	r.Header.Set("webhook-id", id)
	r.Header.Set("webhook-timestamp", ts)
	r.Header.Set("webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return r
}

func TestBillingWebhook(t *testing.T) {
	body := `{"type":"payment.succeeded","data":{"payment_id":"pay_1","metadata":{"workspace_id":"ws-1"}}}`

	t.Run("valid signature is handled", func(t *testing.T) {
		ev := &fakeEvents{}
		h := &BillingHandler{events: ev, webhookSecret: testWebhookSecret}
		w := httptest.NewRecorder()
		h.Webhook(w, signedWebhook(t, body, time.Now()))
		if w.Code != http.StatusOK || len(ev.got) != 1 || ev.got[0].Type != "payment.succeeded" || ev.got[0].Data.PaymentID != "pay_1" {
			t.Fatalf("code=%d events=%+v", w.Code, ev.got)
		}
	})
	t.Run("bad signature is rejected", func(t *testing.T) {
		ev := &fakeEvents{}
		h := &BillingHandler{events: ev, webhookSecret: testWebhookSecret}
		r := signedWebhook(t, body, time.Now())
		r.Header.Set("webhook-signature", "v1,AAAA")
		w := httptest.NewRecorder()
		h.Webhook(w, r)
		if w.Code != http.StatusUnauthorized || len(ev.got) != 0 {
			t.Fatalf("code=%d events=%d", w.Code, len(ev.got))
		}
	})
	t.Run("bad json is a 400", func(t *testing.T) {
		h := &BillingHandler{events: &fakeEvents{}, webhookSecret: testWebhookSecret}
		w := httptest.NewRecorder()
		h.Webhook(w, signedWebhook(t, `{not json`, time.Now()))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code=%d", w.Code)
		}
	})
	t.Run("handler failure asks Dodo to retry", func(t *testing.T) {
		h := &BillingHandler{events: &fakeEvents{err: errors.New("db down")}, webhookSecret: testWebhookSecret}
		w := httptest.NewRecorder()
		h.Webhook(w, signedWebhook(t, body, time.Now()))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("code=%d", w.Code)
		}
	})
	t.Run("billing not configured", func(t *testing.T) {
		h := &BillingHandler{}
		w := httptest.NewRecorder()
		h.Webhook(w, signedWebhook(t, body, time.Now()))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("code=%d", w.Code)
		}
	})
}

func withClaims(r *http.Request, role string) *http.Request {
	return r.WithContext(middleware.WithClaims(r.Context(), &auth.Claims{UserID: "u1", WorkspaceID: "ws-1", Email: "a@b.co", Role: role}))
}

func TestBillingCheckoutGuards(t *testing.T) {
	cases := []struct {
		name string
		h    *BillingHandler
		role string
		body string
		want int
	}{
		{"not configured", &BillingHandler{}, "admin", `{"item":"starter"}`, http.StatusServiceUnavailable},
		{"members can't buy", &BillingHandler{dodo: billing.NewDodo(billing.DodoConfig{APIKey: "k", BaseURL: "http://127.0.0.1:1"})}, "member", `{"item":"starter"}`, http.StatusForbidden},
		{"unknown item", &BillingHandler{dodo: billing.NewDodo(billing.DodoConfig{APIKey: "k", BaseURL: "http://127.0.0.1:1"})}, "admin", `{"item":"enterprise"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		c.h.Checkout(w, withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/billing/checkout", strings.NewReader(c.body)), c.role))
		if w.Code != c.want {
			t.Errorf("%s: code=%d want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}
