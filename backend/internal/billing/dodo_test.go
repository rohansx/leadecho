package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDodoCheckout(t *testing.T) {
	var got struct {
		path, auth string
		body       map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.auth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		_, _ = w.Write([]byte(`{"session_id":"cks_1","checkout_url":"https://test.checkout.dodopayments.com/session/cks_1"}`))
	}))
	defer srv.Close()

	d := NewDodo(DodoConfig{APIKey: "sk_test", BaseURL: srv.URL, Products: Products{Starter: "pdt_s", Pro: "pdt_p", Topup: "pdt_t"},
		ReturnURL: "https://leadecho.app/settings?tab=billing"})
	url, err := d.Checkout(context.Background(), CheckoutRequest{Item: "starter", WorkspaceID: "ws-1", Email: "a@b.co", Name: "Ann"})
	if err != nil || url != "https://test.checkout.dodopayments.com/session/cks_1" {
		t.Fatalf("checkout = %q %v", url, err)
	}
	if got.path != "/checkouts" || got.auth != "Bearer sk_test" {
		t.Fatalf("path=%s auth=%s", got.path, got.auth)
	}
	cart := got.body["product_cart"].([]any)[0].(map[string]any)
	meta := got.body["metadata"].(map[string]any)
	cust := got.body["customer"].(map[string]any)
	if cart["product_id"] != "pdt_s" || cart["quantity"] != float64(1) || meta["workspace_id"] != "ws-1" ||
		cust["email"] != "a@b.co" || got.body["return_url"] != "https://leadecho.app/settings?tab=billing" {
		t.Fatalf("body = %+v", got.body)
	}

	// Existing Dodo customer: reuse it instead of email.
	if _, err := d.Checkout(context.Background(), CheckoutRequest{Item: "topup", WorkspaceID: "ws-1", CustomerID: "cus_9", Email: "a@b.co"}); err != nil {
		t.Fatal(err)
	}
	cust = got.body["customer"].(map[string]any)
	if cust["customer_id"] != "cus_9" || cust["email"] != nil || got.body["product_cart"].([]any)[0].(map[string]any)["product_id"] != "pdt_t" {
		t.Fatalf("existing-customer body = %+v", got.body)
	}

	if _, err := d.Checkout(context.Background(), CheckoutRequest{Item: "enterprise", WorkspaceID: "ws-1"}); err == nil {
		t.Fatal("unknown item must error")
	}
}

func TestDodoPortalAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/customers/cus_9/customer-portal/session" && r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"link":"https://test.customer.dodopayments.com/session/abc"}`))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"code":"INVALID","message":"bad product"}`))
	}))
	defer srv.Close()
	d := NewDodo(DodoConfig{APIKey: "k", BaseURL: srv.URL, Products: Products{Starter: "pdt_s"}})

	link, err := d.PortalLink(context.Background(), "cus_9")
	if err != nil || link != "https://test.customer.dodopayments.com/session/abc" {
		t.Fatalf("portal = %q %v", link, err)
	}
	if _, err := d.Checkout(context.Background(), CheckoutRequest{Item: "starter", WorkspaceID: "w"}); err == nil {
		t.Fatal("422 must surface as an error")
	}
	if NewDodo(DodoConfig{}) != nil {
		t.Fatal("no API key: Dodo must be nil (billing off)")
	}
}
