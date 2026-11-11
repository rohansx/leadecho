package treg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

const testWS = "11111111-2222-3333-4444-555555555555"

type fakeLedger struct {
	mu   sync.Mutex
	rows []database.RecordTregCallParams
}

func (f *fakeLedger) RecordTregCall(_ context.Context, p database.RecordTregCallParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, p)
	return nil
}

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *fakeLedger) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	l := &fakeLedger{}
	c := New(Config{Token: "tok", OrgID: "7", BaseURL: srv.URL, MaxCostMicro: 20000}, l, zerolog.Nop())
	c.backoff = func(int) time.Duration { return 0 }
	return c, l
}

func TestNewDisabledWithoutToken(t *testing.T) {
	var c *Client = New(Config{}, nil, zerolog.Nop())
	if c.Enabled() {
		t.Fatal("client without token must be disabled")
	}
	if _, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestCallSetsHeadersAndRecordsLedger(t *testing.T) {
	var got http.Header
	var body map[string]any
	c, l := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if r.Method != http.MethodPost || r.URL.Path != "/call/treg.x.search.posts" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("X-Treg-Cost-Micro", "750")
		w.Header().Set("X-Treg-Call-Id", "call_abc")
		w.Header().Set("X-Treg-Served-By", "anyapi")
		_, _ = w.Write([]byte(`{"output":{"posts":[]}}`))
	})
	resp, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "treg.x.search.posts", Body: map[string]string{"q": "crm"}})
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"X-Treg-Token":          "tok",
		"X-Treg-Meta":           "customer=" + testWS,
		"X-Treg-Route-Max-Cost": "0.02",
		"Content-Type":          "application/json",
	}
	for k, want := range checks {
		if got.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, got.Get(k), want)
		}
	}
	if got.Get("Idempotency-Key") == "" {
		t.Error("Idempotency-Key missing")
	}
	if body["q"] != "crm" {
		t.Errorf("body = %v", body)
	}
	if resp.CallID != "call_abc" || resp.CostMicro != 750 || resp.ServedBy != "anyapi" {
		t.Errorf("resp = %+v", resp)
	}
	if len(l.rows) != 1 {
		t.Fatalf("ledger rows = %d", len(l.rows))
	}
	want := database.RecordTregCallParams{WorkspaceID: testWS, Tool: "treg.x.search.posts", CallID: "call_abc", CostMicro: 750, Status: "ok"}
	if l.rows[0] != want {
		t.Errorf("ledger = %+v, want %+v", l.rows[0], want)
	}
}

func TestCallRetriesWithSameIdempotencyKey(t *testing.T) {
	var keys []string
	var n atomic.Int32
	c, l := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if n.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("X-Treg-Call-Id", "c3")
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t", IdempotencyKey: "fixed-key"}); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || keys[0] != "fixed-key" || keys[1] != keys[0] || keys[2] != keys[0] {
		t.Fatalf("keys = %v", keys)
	}
	if len(l.rows) != 3 || l.rows[0].Status != "http_502" || l.rows[2].Status != "ok" {
		t.Fatalf("ledger = %+v", l.rows)
	}
}

func TestCallGeneratedKeyStableAcrossRetries(t *testing.T) {
	var keys []string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Fatalf("want 503 APIError, got %v", err)
	}
	if len(keys) != maxAttempts || keys[0] == "" || keys[1] != keys[0] {
		t.Fatalf("keys = %v", keys)
	}
}

func TestCallErrorMapping(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   error
		wantCode  string
		wantTries int32
	}{
		{"insufficient balance", 402, `{"error":"insufficient_balance","balance_micro":3,"topup_url":"https://x"}`, ErrInsufficientBalance, "", 1},
		{"platform daily cap", 429, `{"error":"platform_daily_cap_reached"}`, ErrPlatformDailyCap, "", 1},
		{"route max cost", 402, `{"error":"route_max_cost"}`, nil, "route_max_cost", 1},
		// a workspace's own cap is covered in customer_cap_test.go (ErrCustomerDailyCap)
		{"route failed not retried", 502, `{"error":"route_failed","charged_micro":0}`, nil, "route_failed", 1},
		{"provider 400", 400, `{"message":"bad q"}`, nil, "http_400", 1},
		{"5xx retried", 500, `oops`, nil, "http_500", maxAttempts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tries atomic.Int32
			c, l := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				tries.Add(1)
				w.Header().Set("X-Treg-Error", "1")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t"})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				if !IsOperatorError(err) {
					t.Fatal("must be an operator error")
				}
			} else {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Code != tc.wantCode {
					t.Fatalf("want APIError %s, got %v", tc.wantCode, err)
				}
				if IsOperatorError(err) {
					t.Fatal("must not be an operator error")
				}
			}
			if tries.Load() != tc.wantTries {
				t.Fatalf("tries = %d, want %d", tries.Load(), tc.wantTries)
			}
			if int32(len(l.rows)) != tc.wantTries {
				t.Fatalf("ledger rows = %d", len(l.rows))
			}
		})
	}
}

func TestOperatorErrorPausesCalls(t *testing.T) {
	var tries atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		tries.Add(1)
		w.WriteHeader(402)
		_, _ = io.WriteString(w, `{"error":"insufficient_balance"}`)
	})
	_, _ = c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t"})
	_, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t"})
	if !errors.Is(err, ErrPaused) || !IsOperatorError(err) {
		t.Fatalf("want ErrPaused, got %v", err)
	}
	if tries.Load() != 1 {
		t.Fatalf("paused client still called treg (%d)", tries.Load())
	}
}

func TestCallRetriesTimeouts(t *testing.T) {
	var tries atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if tries.Add(1) == 1 {
			time.Sleep(200 * time.Millisecond)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	c.http.Timeout = 50 * time.Millisecond
	if _, err := c.Call(context.Background(), Request{WorkspaceID: testWS, Tool: "t"}); err != nil {
		t.Fatal(err)
	}
	if tries.Load() != 2 {
		t.Fatalf("tries = %d", tries.Load())
	}
}

func TestCallValidatesInputs(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not call treg")
	})
	cases := []Request{
		{WorkspaceID: "", Tool: "t"},
		{WorkspaceID: "a@b.com", Tool: "t"},
		{WorkspaceID: "has space", Tool: "t"},
		{WorkspaceID: testWS, Tool: ""},
		{WorkspaceID: testWS, Tool: "../orgs/1"},
	}
	for _, r := range cases {
		if _, err := c.Call(context.Background(), r); err == nil {
			t.Errorf("want error for %+v", r)
		}
	}
}

func TestMaxCostHeaderFormat(t *testing.T) {
	cases := map[int64]string{20000: "0.02", 1500: "0.0015", 1000000: "1", 1: "0.000001"}
	for micro, want := range cases {
		if got := microToUSD(micro); got != want {
			t.Errorf("microToUSD(%d) = %q, want %q", micro, got, want)
		}
	}
}
