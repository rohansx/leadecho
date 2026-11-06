package treg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
)

func TestSetCustomerDailyBudget(t *testing.T) {
	var gotPath, gotMethod, gotTok string
	var body map[string]int64
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotTok = r.URL.Path, r.Method, r.Header.Get("X-Treg-Token")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.SetCustomerDailyBudget(context.Background(), testWS, 5000000); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/orgs/7/budgets/customer/"+testWS || gotTok != "tok" {
		t.Fatalf("%s %s tok=%s", gotMethod, gotPath, gotTok)
	}
	if body["daily_cap_micro"] != 5000000 {
		t.Fatalf("body = %v", body)
	}
}

func TestSetCustomerDailyBudgetErrors(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"admin_required"}`))
	})
	var apiErr *APIError
	if err := c.SetCustomerDailyBudget(context.Background(), testWS, 1); !errors.As(err, &apiErr) || apiErr.Code != "admin_required" {
		t.Fatalf("got %v", err)
	}
	if err := c.SetCustomerDailyBudget(context.Background(), "bad id", 1); err == nil {
		t.Fatal("want validation error")
	}
	if err := c.SetCustomerDailyBudget(context.Background(), testWS, -1); err == nil {
		t.Fatal("want validation error for negative cap")
	}
	noOrg := New(Config{Token: "t"}, nil, zerolog.Nop())
	if err := noOrg.SetCustomerDailyBudget(context.Background(), testWS, 1); !errors.Is(err, ErrNoOrg) {
		t.Fatalf("want ErrNoOrg, got %v", err)
	}
}

func TestUsageByTag(t *testing.T) {
	var gotQuery string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/7/usage/by-tag" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"key":"customer","days":30,"rows":[{"value":"` + testWS + `","charged_micro":41234,"charged_usd":0.041234,"calls":22}],"attributed_micro":41234,"unattributed_micro":1880,"total_micro":43114}`))
	})
	u, err := c.UsageByTag(context.Background(), "customer", 30)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "days=30&key=customer" {
		t.Errorf("query = %s", gotQuery)
	}
	if len(u.Rows) != 1 || u.Rows[0].Value != testWS || u.Rows[0].ChargedMicro != 41234 || u.Rows[0].Calls != 22 {
		t.Fatalf("rows = %+v", u.Rows)
	}
	if !u.Reconciled() {
		t.Fatal("41234+1880 == 43114 must reconcile")
	}
	u.TotalMicro++
	if u.Reconciled() {
		t.Fatal("mismatch must not reconcile")
	}
}

func TestBalance(t *testing.T) {
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"org_id":7,"balance_micro":9824500,"balance_usd":9.8245}`))
	})
	micro, err := c.Balance(context.Background())
	if err != nil || micro != 9824500 || gotPath != "/orgs/7/balance" {
		t.Fatalf("balance = %d %v path=%s", micro, err, gotPath)
	}
}
