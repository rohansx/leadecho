package middleware

import (
	"net/http"
	"sync"
	"time"
)

// Quota is a fixed-window per-key counter guarding endpoints that spend the
// operator's LLM or treg budget (chat, onboarding analysis, engagers, ...).
// ponytail: in-process, so limits are per replica and reset on restart; move
// to the Redis auth.Counter if the API runs more than a couple of replicas.
type Quota struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	seen map[string]quotaWindow
}

type quotaWindow struct {
	start time.Time
	n     int
}

func NewQuota(limit int, window time.Duration) *Quota {
	return &Quota{limit: limit, window: window, now: time.Now, seen: map[string]quotaWindow{}}
}

// Allow records one call for key and reports whether it fits the window.
func (q *Quota) Allow(key string) bool {
	now := q.now()
	start := now.Truncate(q.window)
	q.mu.Lock()
	defer q.mu.Unlock()
	w := q.seen[key]
	if !w.start.Equal(start) {
		w = quotaWindow{start: start}
	}
	if w.n >= q.limit {
		return false
	}
	q.seen[key] = quotaWindow{start: start, n: w.n + 1}
	return true
}

// WorkspaceQuota answers 429 once the caller's workspace exceeds q.
func WorkspaceQuota(q *Quota) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !q.Allow(WorkspaceID(r.Context())) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many requests for this workspace, try again shortly"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
