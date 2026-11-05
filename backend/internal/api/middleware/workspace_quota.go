package middleware

import (
	"context"
	"strconv"

	"leadecho/internal/auth"
	"net/http"
	"sync"
	"time"
)

// Quota is a fixed-window per-key counter guarding endpoints that spend the
// operator's LLM or treg budget (chat, onboarding analysis, engagers, ...).
// With a shared counter (Redis) the budget spans replicas and restarts; the
// in-process window is the fallback when there is no Redis or it errors.
type Quota struct {
	limit  int
	window time.Duration
	now    func() time.Time
	shared auth.Counter
	name   string

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

// Shared makes the quota use c (e.g. auth.NewRedisCounter) under name.
func (q *Quota) Shared(c auth.Counter, name string) *Quota {
	q.shared, q.name = c, name
	return q
}

// Allow records one call for key and reports whether it fits the window.
func (q *Quota) Allow(ctx context.Context, key string) bool {
	ok, _ := q.allow(ctx, key)
	return ok
}

// allow also returns how long until the window resets.
func (q *Quota) allow(ctx context.Context, key string) (bool, time.Duration) {
	if q.shared != nil {
		if n, ttl, err := q.shared.Incr(ctx, "quota:"+q.name+":"+key, q.window); err == nil {
			return n <= int64(q.limit), ttl
		}
	}
	ok := q.allowLocal(key)
	start := q.now().Truncate(q.window)
	return ok, start.Add(q.window).Sub(q.now())
}

// maxLocalKeys bounds the fallback map; stale windows are pruned past it.
const maxLocalKeys = 10_000

func (q *Quota) allowLocal(key string) bool {
	now := q.now()
	start := now.Truncate(q.window)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.seen) > maxLocalKeys {
		for k, w := range q.seen {
			if !w.start.Equal(start) {
				delete(q.seen, k)
			}
		}
	}
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
			if ok, wait := q.allow(r.Context(), WorkspaceID(r.Context())); !ok {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many requests for this workspace, try again shortly"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
