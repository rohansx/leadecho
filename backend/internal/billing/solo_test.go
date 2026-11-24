package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSoloPlan(t *testing.T) {
	p, ok := PlanByID("solo")
	if !ok || p.PriceCents != 1900 || p.MonthlyCredits != 1500 || p.KeywordLimit != 5 || p.PaidData {
		t.Fatalf("solo = %+v %v", p, ok)
	}
	for _, id := range []string{"starter", "pro"} {
		if p, _ := PlanByID(id); !p.PaidData {
			t.Fatalf("%s must include paid data", id)
		}
	}
	if p, _ := PlanByID("free"); p.PaidData {
		t.Fatal("free must not include paid data")
	}
}

func TestProductsPlanSolo(t *testing.T) {
	ps := Products{Solo: "pdt_solo", Starter: "pdt_s", Pro: "pdt_p", Topup: "pdt_t"}
	for product, want := range map[string]string{"pdt_solo": "solo", "pdt_s": "starter", "pdt_p": "pro"} {
		if p, ok := ps.plan(product); !ok || p.ID != want {
			t.Fatalf("plan(%s) = %s %v, want %s", product, p.ID, ok, want)
		}
	}
	// Solo not configured: an empty product id must never map to a plan.
	unset := Products{Starter: "pdt_s"}
	if _, ok := unset.plan(""); ok {
		t.Fatal("empty product id matched a plan")
	}
	if _, ok := ps.plan("pdt_t"); ok {
		t.Fatal("top-up is not a plan")
	}
}

func TestDodoSells(t *testing.T) {
	var nilDodo *Dodo
	if nilDodo.Sells("solo") {
		t.Fatal("billing off sells nothing")
	}
	d := NewDodo(DodoConfig{APIKey: "k", Products: Products{Starter: "pdt_s", Pro: "pdt_p"}})
	if d.Sells("solo") || !d.Sells("starter") || d.Sells("topup") || d.Sells("enterprise") {
		t.Fatal("Sells must follow configured products")
	}
	if _, err := d.Checkout(context.Background(), CheckoutRequest{Item: "solo", WorkspaceID: "w"}); err == nil {
		t.Fatal("solo checkout without a product must error, not call Dodo")
	}
}

func TestDodoCheckoutSolo(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"checkout_url":"https://x/cks"}`))
	}))
	defer srv.Close()
	d := NewDodo(DodoConfig{APIKey: "k", BaseURL: srv.URL, Products: Products{Solo: "pdt_solo"}})
	if _, err := d.Checkout(context.Background(), CheckoutRequest{Item: "solo", WorkspaceID: "w"}); err != nil {
		t.Fatal(err)
	}
	if body["product_cart"].([]any)[0].(map[string]any)["product_id"] != "pdt_solo" {
		t.Fatalf("body = %+v", body)
	}
}
