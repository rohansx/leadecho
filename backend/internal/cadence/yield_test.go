package cadence

import (
	"testing"
	"time"
)

func TestYieldFlags(t *testing.T) {
	tests := []struct {
		name               string
		y                  Yield
		throttled, lowFlag bool
	}{
		{"99 posts, no lead", Yield{Posts: 99}, false, false},
		{"100 posts, no lead", Yield{Posts: 100}, true, false},
		{"149 posts, no lead", Yield{Posts: 149}, true, false},
		{"150 posts, no lead", Yield{Posts: 150}, true, true},
		{"535 posts, no lead, some asks", Yield{Posts: 535, Passed: 40, Asks: 12}, true, true},
		{"one lead lifts both", Yield{Posts: 535, Leads: 1}, false, false},
		{"nothing fetched", Yield{}, false, false},
	}
	for _, tt := range tests {
		if got := tt.y.Throttled(); got != tt.throttled {
			t.Errorf("%s: Throttled = %v, want %v", tt.name, got, tt.throttled)
		}
		if got := tt.y.LowYield(); got != tt.lowFlag {
			t.Errorf("%s: LowYield = %v, want %v", tt.name, got, tt.lowFlag)
		}
	}
	sum := Yield{Posts: 80, Passed: 5, Leads: 0, Asks: 1}.Add(Yield{Posts: 70, Passed: 2, Leads: 0, Asks: 3})
	if sum != (Yield{Posts: 150, Passed: 7, Asks: 4}) || !sum.LowYield() {
		t.Errorf("sum = %+v", sum)
	}
}

// A throttled platform is due once a day whatever the customer set; their
// stored cadence is left as it was, and other platforms keep theirs.
func TestThrottle(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	stored := Map{"hackernews": "15m", "reddit": "30m"}
	got := stored.Throttle([]string{"hackernews", "bluesky"})
	if got.Interval("hackernews") != 24*time.Hour || got.Interval("bluesky") != 24*time.Hour || got.Interval("reddit") != 30*time.Minute {
		t.Fatalf("throttled cadence = %v", got)
	}
	if stored["hackernews"] != "15m" || len(stored) != 2 {
		t.Fatalf("the stored cadence was changed: %v", stored)
	}
	last := map[string]time.Time{"hackernews": now.Add(-5 * time.Hour), "bluesky": now.Add(-25 * time.Hour), "reddit": now.Add(-time.Hour)}
	due := Due([]string{"hackernews", "bluesky", "reddit"}, got, last, now)
	if len(due) != 2 || due[0] != "bluesky" || due[1] != "reddit" {
		t.Fatalf("due = %v, want bluesky (a day old) and reddit (its own cadence)", due)
	}
	var none Map
	if got := none.Throttle([]string{"devto"}); got.Interval("devto") != 24*time.Hour {
		t.Fatalf("nil cadence: %v", got)
	}
}
