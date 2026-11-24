package platform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

const day = 24 * time.Hour

func TestFeatureInUse(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	old := now.Add(-90 * day)
	cases := []struct {
		name          string
		last, created time.Time
		want          bool
	}{
		{"new workspace, day 1, never opened", time.Time{}, now.Add(-time.Hour), true},
		{"new workspace, exactly 14 days old, never opened", time.Time{}, now.Add(-FeatureIdleAfter), true},
		{"workspace day 15, never opened", time.Time{}, now.Add(-15 * day), false},
		{"old workspace, opened yesterday", now.Add(-day), old, true},
		{"old workspace, opened exactly 14 days ago", now.Add(-FeatureIdleAfter), old, true},
		{"old workspace, opened 15 days ago", now.Add(-15 * day), old, false},
		{"nothing known", time.Time{}, time.Time{}, false},
	}
	for _, c := range cases {
		if got := FeatureInUse(c.last, c.created, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFeatureActiveFailsOpen(t *testing.T) {
	now := time.Now()
	lookup := func(last, created time.Time, err error) featureLookup {
		return func(context.Context, string, string) (time.Time, time.Time, error) { return last, created, err }
	}
	ctx := context.Background()
	if !featureActive(ctx, lookup(time.Time{}, time.Time{}, errors.New("db down")), zerolog.Nop(), "ws", "demand", now) {
		t.Fatal("a failed lookup must run the job")
	}
	if featureActive(ctx, lookup(time.Time{}, now.Add(-30*day), nil), zerolog.Nop(), "ws", "demand", now) {
		t.Fatal("an idle workspace must be skipped")
	}
	if !featureActive(ctx, lookup(now.Add(-day), now.Add(-30*day), nil), zerolog.Nop(), "ws", "demand", now) {
		t.Fatal("a workspace opened yesterday must run")
	}
}
