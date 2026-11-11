package treg

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Body captured from a live refusal (2026-11-11).
const capBody = `{"detail":{"error":"tag_spend_cap_reached","dim":"customer","val":"ws","spent_micro":199351,"cap_micro":200000,"period":"day","charged_micro":0}}`

func TestCustomerDailyCapStopsOnlyThatWorkspace(t *testing.T) {
	const otherWS = "99999999-2222-3333-4444-555555555555"
	var hits atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("X-Treg-Meta") == "customer="+testWS {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(capBody))
			return
		}
		_, _ = w.Write([]byte(`{"output":{}}`))
	})
	ctx := context.Background()

	if _, err := c.Call(ctx, Request{WorkspaceID: testWS, Tool: "treg.x.search.posts"}); !errors.Is(err, ErrCustomerDailyCap) {
		t.Fatalf("want ErrCustomerDailyCap, got %v", err)
	}
	if IsOperatorError(ErrCustomerDailyCap) {
		t.Fatal("one workspace's cap is not an account-level problem")
	}
	// The capped workspace makes no further HTTP calls today.
	before := hits.Load()
	if _, err := c.Call(ctx, Request{WorkspaceID: testWS, Tool: "treg.x.search.posts"}); !errors.Is(err, ErrCustomerDailyCap) {
		t.Fatalf("second call: want ErrCustomerDailyCap, got %v", err)
	}
	if hits.Load() != before {
		t.Fatal("a capped workspace must not keep calling treg")
	}
	// Everyone else carries on.
	if _, err := c.Call(ctx, Request{WorkspaceID: otherWS, Tool: "treg.x.search.posts"}); err != nil {
		t.Fatalf("other workspace should be unaffected: %v", err)
	}
	// The cap lifts once its time has passed.
	c.capped.Store(testWS, time.Now().Add(-time.Minute))
	if _, err := c.Call(ctx, Request{WorkspaceID: testWS, Tool: "treg.x.search.posts"}); !errors.Is(err, ErrCustomerDailyCap) || hits.Load() == before+1 {
		t.Fatalf("after the reset the call must go out again (and here be refused again): err=%v hits=%d", err, hits.Load())
	}
}

func TestErrorCodeAndMidnight(t *testing.T) {
	if got := errorCode([]byte(capBody)); got != "tag_spend_cap_reached" {
		t.Fatalf("nested code: got %q", got)
	}
	if got := errorCode([]byte(`{"error":"insufficient_balance"}`)); got != "insufficient_balance" {
		t.Fatalf("flat code: got %q", got)
	}
	now := time.Date(2026, 11, 11, 23, 59, 0, 0, time.FixedZone("x", 5*3600)) // 18:59 UTC
	if got := nextUTCMidnight(now); !got.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}
