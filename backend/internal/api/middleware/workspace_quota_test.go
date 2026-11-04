package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"leadecho/internal/auth"
)

func TestQuotaAllow(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	q := NewQuota(2, time.Minute)
	q.now = func() time.Time { return now }
	if !q.Allow("ws1") || !q.Allow("ws1") {
		t.Fatal("first two calls must pass")
	}
	if q.Allow("ws1") {
		t.Fatal("third call in window must be refused")
	}
	if !q.Allow("ws2") {
		t.Fatal("other workspace has its own budget")
	}
	now = now.Add(time.Minute)
	if !q.Allow("ws1") {
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
