package treg

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *memCache) Set(_ context.Context, key string, val []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = val
}

// Same tool + query within the TTL is served from the cache: no treg call,
// no credit, regardless of which workspace asks (the data is public).
func TestCallCachesResults(t *testing.T) {
	var hits atomic.Int32
	status := http.StatusOK
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		w.Header().Set("X-Treg-Cost-Micro", "750")
		_, _ = w.Write([]byte(`{"output":{"posts":[{"id":"1"}]}}`))
	})
	m := &fakeMeter{}
	c.SetMeter(m)
	c.SetCache(&memCache{m: map[string][]byte{}}, 4*time.Hour)
	ctx := context.Background()
	q := Request{WorkspaceID: meterWS, Tool: "treg.x.search.posts", Body: map[string]string{"q": "mcp proxy"}}

	r1, err := c.Call(ctx, q)
	if err != nil || hits.Load() != 1 {
		t.Fatalf("first call: err=%v hits=%d", err, hits.Load())
	}
	other := q
	other.WorkspaceID = "99999999-2222-3333-4444-555555555555"
	r2, err := c.Call(ctx, other)
	if err != nil || hits.Load() != 1 || string(r2.Body) != string(r1.Body) || r2.CostMicro != 0 || r2.ServedBy != "cache" {
		t.Fatalf("cached call: err=%v hits=%d resp=%+v", err, hits.Load(), r2)
	}
	if m.spent != 1 {
		t.Fatalf("spent=%d, want 1 (cache hits are free)", m.spent)
	}

	diff := q
	diff.Body = map[string]string{"q": "agent audit log"}
	if _, err := c.Call(ctx, diff); err != nil || hits.Load() != 2 {
		t.Fatalf("different query must call treg: err=%v hits=%d", err, hits.Load())
	}

	status = http.StatusBadRequest
	fail := q
	fail.Body = map[string]string{"q": "fails"}
	_, _ = c.Call(ctx, fail)
	before := hits.Load()
	_, _ = c.Call(ctx, fail)
	if hits.Load() == before {
		t.Fatal("failed calls must not be cached")
	}
}
