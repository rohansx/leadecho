package monitor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestForEachConcurrentBoundsAndCompletes(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()}
	var inFlight, peak, calls atomic.Int32
	m.forEachConcurrent(context.Background(), 20, func(i int) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		calls.Add(1)
	})
	if calls.Load() != 20 {
		t.Fatalf("calls = %d, want 20", calls.Load())
	}
	if peak.Load() > scoringConcurrency {
		t.Fatalf("peak concurrency %d exceeds %d", peak.Load(), scoringConcurrency)
	}
}

func TestForEachConcurrentRecoversPanics(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()}
	var calls atomic.Int32
	m.forEachConcurrent(context.Background(), 5, func(i int) {
		calls.Add(1)
		if i == 2 {
			panic("boom")
		}
	})
	if calls.Load() != 5 {
		t.Fatalf("calls = %d, want 5 (a panic must not stop the others)", calls.Load())
	}
}
