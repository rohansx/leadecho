package platform

import (
	"testing"
	"time"
)

func TestFeatureInUse(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never opened", time.Time{}, false},
		{"opened an hour ago", now.Add(-time.Hour), true},
		{"opened exactly 14 days ago", now.Add(-FeatureIdleAfter), true},
		{"opened 15 days ago", now.Add(-15 * 24 * time.Hour), false},
	}
	for _, c := range cases {
		if got := FeatureInUse(c.last, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
