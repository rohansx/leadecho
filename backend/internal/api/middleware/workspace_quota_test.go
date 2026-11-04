package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"leadecho/internal/auth"
)

func TestQuotaAllow(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	q := NewQuota(2, time.Minute)
	q.now = func() time.Time { return now }
	if !q.Allow(ctx, "ws1") || !q.Allow(ctx, "ws1") {
		t.Fatal("first two calls must pass")
	}
	if q.Allow(ctx, "ws1") {
		t.Fatal("third call in window must be refused")
	}
	if !q.Allow(ctx, "ws2") {
		t.Fatal("other workspace has its own budget")
	}
	now = now.Add(time.Minute)
	if !q.Allow(ctx, "ws1") {
		t.Fatal("new window must reset the budget")
	}
}

func TestWorkspaceQuotaMiddleware(t *testing.T) {
	q := NewQuota(1, time.Hour)
	h := WorkspaceQuota(q)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	do := func() int {
		req := httptest.NewRequest("POST", "/chat", nil)
		req = req.WithContext(WithClaims(req.Context(), &auth.Claims{WorkspaceID: "ws1"}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := do(); c != 204 {
		t.Fatalf("first = %d", c)
	}
	if c := do(); c != http.StatusTooManyRequests {
		t.Fatalf("second = %d, want 429", c)
	}
}

// fakeCounter is a shared (Redis-like) counter; fail makes every call error.
type fakeCounter struct {
	n    map[string]int64
	fail bool
}

func (f *fakeCounter) Incr(_ context.Context, key string, _ time.Duration) (int64, time.Duration, error) {
	if f.fail {
		return 0, 0, errors.New("redis down")
	}
	f.n[key]++
	return f.n[key], time.Minute, nil
}
func (f *fakeCounter) Reset(context.Context, string) error { return nil }

func TestQuotaSharedCounter(t *testing.T) {
	ctx := context.Background()
	shared := &fakeCounter{n: map[string]int64{}}
	a, b := NewQuota(2, time.Minute).Shared(shared, "chat"), NewQuota(2, time.Minute).Shared(shared, "chat")
	// Two replicas share one budget.
	if !a.Allow(ctx, "ws") || !b.Allow(ctx, "ws") || a.Allow(ctx, "ws") {
		t.Fatal("replicas must share the workspace budget")
	}
	// Redis errors fall back to the local window instead of failing open.
	down := NewQuota(1, time.Minute).Shared(&fakeCounter{fail: true}, "chat")
	if !down.Allow(ctx, "ws") || down.Allow(ctx, "ws") {
		t.Fatal("with redis down the local limit must still apply")
	}
}
