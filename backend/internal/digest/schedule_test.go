package digest

import (
	"strings"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestScheduleDue(t *testing.T) {
	base := DefaultSchedule() // daily 09:30, weekly monday, UTC
	ny := base
	ny.Timezone = "America/New_York"
	off := base
	off.DailyEnabled, off.WeeklyEnabled = false, false
	sunday := base
	sunday.WeeklyDay = "sunday"

	tests := []struct {
		name      string
		s         Schedule
		kind      Kind
		now       string
		wantDue   bool
		wantStart string
		wantEnd   string
	}{
		{"daily just after send time", base, KindDaily, "2026-11-04T09:31:00Z", true, "2026-11-03T09:30:00Z", "2026-11-04T09:30:00Z"},
		{"daily exactly at send time", base, KindDaily, "2026-11-04T09:30:00Z", true, "2026-11-03T09:30:00Z", "2026-11-04T09:30:00Z"},
		{"daily before send time is not due", base, KindDaily, "2026-11-04T09:29:00Z", false, "2026-11-02T09:30:00Z", "2026-11-03T09:30:00Z"},
		{"daily stale beyond window", base, KindDaily, "2026-11-04T20:00:00Z", false, "2026-11-03T09:30:00Z", "2026-11-04T09:30:00Z"},
		{"daily disabled", off, KindDaily, "2026-11-04T09:31:00Z", false, "2026-11-03T09:30:00Z", "2026-11-04T09:30:00Z"},
		// 09:30 New York (EST, UTC-5 after Nov 1) = 14:30Z
		{"timezone aware", ny, KindDaily, "2026-11-04T14:35:00Z", true, "2026-11-03T14:30:00Z", "2026-11-04T14:30:00Z"},
		{"timezone aware: 09:31 UTC is early morning in NY", ny, KindDaily, "2026-11-04T09:31:00Z", false, "2026-11-02T14:30:00Z", "2026-11-03T14:30:00Z"},
		// DST end in NY is 2026-11-01: previous day's 09:30 EDT was 13:30Z.
		{"dst transition keeps local wall clock", ny, KindDaily, "2026-11-01T14:31:00Z", true, "2026-10-31T13:30:00Z", "2026-11-01T14:30:00Z"},
		// 2026-11-02 is a Monday.
		{"weekly on its day", base, KindWeekly, "2026-11-02T09:40:00Z", true, "2026-10-26T09:30:00Z", "2026-11-02T09:30:00Z"},
		{"weekly off day not due", base, KindWeekly, "2026-11-04T09:40:00Z", false, "2026-10-26T09:30:00Z", "2026-11-02T09:30:00Z"},
		{"weekly on its day before time", base, KindWeekly, "2026-11-02T08:00:00Z", false, "2026-10-19T09:30:00Z", "2026-10-26T09:30:00Z"},
		{"weekly sunday", sunday, KindWeekly, "2026-11-08T10:00:00Z", true, "2026-11-01T09:30:00Z", "2026-11-08T09:30:00Z"},
		{"weekly disabled", off, KindWeekly, "2026-11-02T09:40:00Z", false, "2026-10-26T09:30:00Z", "2026-11-02T09:30:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, due := tc.s.Due(tc.kind, mustTime(t, tc.now))
			if due != tc.wantDue {
				t.Fatalf("due = %v, want %v", due, tc.wantDue)
			}
			if !p.Start.Equal(mustTime(t, tc.wantStart)) || !p.End.Equal(mustTime(t, tc.wantEnd)) {
				t.Fatalf("period = %s → %s, want %s → %s", p.Start.UTC().Format(time.RFC3339), p.End.UTC().Format(time.RFC3339), tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestScheduleNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      Schedule
		wantErr string
		check   func(t *testing.T, s Schedule)
	}{
		{name: "defaults are valid", in: DefaultSchedule()},
		{name: "normalizes case, whitespace and duplicates", in: Schedule{
			DailyTime: " 7:05 ", WeeklyDay: "Friday", Timezone: "Europe/Berlin",
			Recipients: []string{" A@Example.com", "a@example.com", ""}, Channels: []string{"Slack", "email", "slack"},
		}, check: func(t *testing.T, s Schedule) {
			if s.DailyTime != "07:05" || s.WeeklyDay != "friday" {
				t.Fatalf("got time=%q day=%q", s.DailyTime, s.WeeklyDay)
			}
			if strings.Join(s.Recipients, ",") != "a@example.com" || strings.Join(s.Channels, ",") != "slack,email" {
				t.Fatalf("got recipients=%v channels=%v", s.Recipients, s.Channels)
			}
		}},
		{name: "empty timezone means UTC", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday"}, check: func(t *testing.T, s Schedule) {
			if s.Timezone != "UTC" {
				t.Fatalf("timezone = %q", s.Timezone)
			}
		}},
		{name: "bad time", in: Schedule{DailyTime: "25:00", WeeklyDay: "monday"}, wantErr: "daily_time"},
		{name: "bad time format", in: Schedule{DailyTime: "9am", WeeklyDay: "monday"}, wantErr: "daily_time"},
		{name: "bad day", in: Schedule{DailyTime: "09:30", WeeklyDay: "funday"}, wantErr: "weekly_day"},
		{name: "bad timezone", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday", Timezone: "Mars/Olympus"}, wantErr: "timezone"},
		{name: "bad email", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday", Recipients: []string{"not-an-email"}}, wantErr: "recipient"},
		{name: "display-name email rejected", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday", Recipients: []string{"Bob <b@x.io>"}}, wantErr: "recipient"},
		{name: "too many recipients", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday", Recipients: strings.Split("a@x.io,b@x.io,c@x.io,d@x.io,e@x.io,f@x.io,g@x.io,h@x.io,i@x.io,j@x.io,k@x.io", ",")}, wantErr: "recipients"},
		{name: "bad channel", in: Schedule{DailyTime: "09:30", WeeklyDay: "monday", Channels: []string{"discord"}}, wantErr: "channel"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Normalize()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}
