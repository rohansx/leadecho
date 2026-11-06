package treg

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

type fakeMeter struct {
	spent, refunded int
	err             error
}

func (m *fakeMeter) Spend(_ context.Context, _ string, cost int, _ string) error {
	if m.err != nil {
		return m.err
	}
	m.spent += cost
	return nil
}

func (m *fakeMeter) Refund(_ context.Context, _ string, n int, _ string) { m.refunded += n }

const meterWS = "11111111-2222-3333-4444-555555555555"

// Every paid treg call costs the workspace one credit; calls treg doesn't
// charge for (misses) or that fail are refunded; an empty balance blocks the
// call before any request goes out.
func TestCallMetersCredits(t *testing.T) {
	t.Run("paid hit is charged", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Treg-Cost-Micro", "750")
			_, _ = w.Write([]byte(`{"output":{"posts":[{"id":"1"}]}}`))
		})
		m := &fakeMeter{}
		c.SetMeter(m)
		if _, err := c.Call(context.Background(), Request{WorkspaceID: meterWS, Tool: "treg.x.search.posts"}); err != nil {
			t.Fatal(err)
		}
		if m.spent != 1 || m.refunded != 0 {
			t.Fatalf("spent=%d refunded=%d, want 1/0", m.spent, m.refunded)
		}
	})
	t.Run("free miss is refunded", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Treg-Cost-Micro", "0")
			_, _ = w.Write([]byte(`{"output":{"posts":[]}}`))
		})
		m := &fakeMeter{}
		c.SetMeter(m)
		if _, err := c.Call(context.Background(), Request{WorkspaceID: meterWS, Tool: "treg.x.search.posts"}); err != nil {
			t.Fatal(err)
		}
		if m.spent != 1 || m.refunded != 1 {
			t.Fatalf("spent=%d refunded=%d, want 1/1", m.spent, m.refunded)
		}
	})
	t.Run("failure is refunded", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})
		m := &fakeMeter{}
		c.SetMeter(m)
		if _, err := c.Call(context.Background(), Request{WorkspaceID: meterWS, Tool: "treg.x.search.posts"}); err == nil {
			t.Fatal("want error")
		}
		if m.refunded != 1 {
			t.Fatalf("refunded=%d, want 1", m.refunded)
		}
	})
	t.Run("no credits: no request", func(t *testing.T) {
		var hits atomic.Int32
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
		noCredits := errors.New("out of credits")
		c.SetMeter(&fakeMeter{err: noCredits})
		if _, err := c.Call(context.Background(), Request{WorkspaceID: meterWS, Tool: "treg.x.search.posts"}); !errors.Is(err, noCredits) {
			t.Fatalf("err = %v, want the meter's error", err)
		}
		if hits.Load() != 0 {
			t.Fatalf("treg was called %d times without credits", hits.Load())
		}
	})
}
