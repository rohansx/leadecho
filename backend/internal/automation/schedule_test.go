package automation

import (
	"testing"
	"time"
)

func TestScheduleNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      Schedule
		want    Schedule
		wantErr bool
	}{
		{"defaults", Schedule{}, Schedule{Frequency: "daily", Time: "09:30", Weekday: "monday", Timezone: "UTC"}, false},
		{"weekly cleaned", Schedule{Frequency: " Weekly ", Time: "7:05", Weekday: "Friday", Timezone: "Europe/Berlin"},
			Schedule{Frequency: "weekly", Time: "07:05", Weekday: "friday", Timezone: "Europe/Berlin"}, false},
		{"bad frequency", Schedule{Frequency: "hourly"}, Schedule{}, true},
		{"bad time", Schedule{Time: "25:00"}, Schedule{}, true},
		{"bad weekday", Schedule{Frequency: "weekly", Weekday: "funday"}, Schedule{}, true},
		{"bad tz", Schedule{Timezone: "Mars/Olympus"}, Schedule{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Normalize()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScheduleDue(t *testing.T) {
	mustTime := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	daily := Schedule{Frequency: "daily", Time: "09:30", Weekday: "monday", Timezone: "UTC"}
	weekly := Schedule{Frequency: "weekly", Time: "08:00", Weekday: "wednesday", Timezone: "America/New_York"}
	tests := []struct {
		name      string
		s         Schedule
		now       string
		wantDue   bool
		wantStart string // period start (the occurrence that is due)
	}{
		{"daily before time: yesterday's slot is stale", daily, "2026-11-04T09:00:00Z", false, "2026-11-03T09:30:00Z"},
		{"daily just after time", daily, "2026-11-04T09:31:00Z", true, "2026-11-04T09:30:00Z"},
		{"daily well past window", daily, "2026-11-04T16:00:00Z", false, "2026-11-04T09:30:00Z"},
		// 2026-11-04 is a Wednesday; 08:00 EST = 13:00Z
		{"weekly on day after time", weekly, "2026-11-04T13:05:00Z", true, "2026-11-04T13:00:00Z"},
		{"weekly other day", weekly, "2026-11-05T13:05:00Z", false, "2026-11-04T13:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, due := tt.s.Due(mustTime(tt.now))
			if due != tt.wantDue {
				t.Fatalf("due = %v, want %v", due, tt.wantDue)
			}
			if !start.Equal(mustTime(tt.wantStart)) {
				t.Fatalf("start = %v, want %s", start, tt.wantStart)
			}
		})
	}
	// idempotent per period: every tick inside the window maps to the same slot
	a, _ := daily.Due(mustTime("2026-11-04T09:35:00Z"))
	b, _ := daily.Due(mustTime("2026-11-04T12:00:00Z"))
	if !a.Equal(b) {
		t.Fatalf("ticks in one period must share a slot: %v vs %v", a, b)
	}
}

func TestScheduleLookback(t *testing.T) {
	if got := (Schedule{Frequency: "weekly"}).Lookback(); got != 7*24*time.Hour {
		t.Fatalf("weekly lookback = %v", got)
	}
	if got := (Schedule{Frequency: "daily"}).Lookback(); got != 24*time.Hour {
		t.Fatalf("daily lookback = %v", got)
	}
}
