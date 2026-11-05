// Package automation runs scheduled, deterministic playbooks (daily triage,
// lead sweep, attribution review, custom tool chains) over internal/tools
// and delivers their output in-app and optionally by email (P2 #17).
package automation

import (
	"errors"
	"strings"
	"time"

	"leadecho/internal/digest"
)

// Schedule is when an automation runs: daily or weekly at a local time.
type Schedule struct {
	Frequency string `json:"frequency"` // daily | weekly
	Time      string `json:"time"`      // HH:MM, local to Timezone
	Weekday   string `json:"weekday"`   // weekly only, lowercase English
	Timezone  string `json:"timezone"`  // IANA name
}

// digestSchedule maps onto the digest schedule, whose clock/weekday/DST
// handling is reused for validation and due computation.
func (s Schedule) digestSchedule() digest.Schedule {
	return digest.Schedule{
		DailyEnabled: s.Frequency == "daily", DailyTime: s.Time,
		WeeklyEnabled: s.Frequency == "weekly", WeeklyDay: s.Weekday, Timezone: s.Timezone,
	}
}

func (s Schedule) kind() digest.Kind {
	if s.Frequency == "weekly" {
		return digest.KindWeekly
	}
	return digest.KindDaily
}

// Normalize validates the schedule and returns a cleaned copy with defaults.
func (s Schedule) Normalize() (Schedule, error) {
	out := Schedule{
		Frequency: strings.ToLower(strings.TrimSpace(s.Frequency)),
		Time:      strings.TrimSpace(s.Time),
		Weekday:   strings.TrimSpace(s.Weekday),
		Timezone:  s.Timezone,
	}
	if out.Frequency == "" {
		out.Frequency = "daily"
	}
	if out.Frequency != "daily" && out.Frequency != "weekly" {
		return Schedule{}, errors.New("frequency must be daily or weekly")
	}
	if out.Time == "" {
		out.Time = "09:30"
	}
	if out.Weekday == "" {
		out.Weekday = "monday"
	}
	d, err := out.digestSchedule().Normalize()
	if err != nil {
		msg := strings.NewReplacer("daily_time", "time", "weekly_day", "weekday").Replace(err.Error())
		return Schedule{}, errors.New(msg)
	}
	out.Time, out.Weekday, out.Timezone = d.DailyTime, d.WeeklyDay, d.Timezone
	return out, nil
}

// Due returns the most recent scheduled occurrence at or before now and
// whether it should run now (within digest.DueWindow). The occurrence is the
// idempotency key: every tick in one period maps to the same value.
func (s Schedule) Due(now time.Time) (time.Time, bool) {
	p, due := s.digestSchedule().Due(s.kind(), now)
	return p.End, due
}

// MinGap is the least time between two scheduled runs (see digest.MinGap).
func MinGap(s Schedule) time.Duration { return digest.MinGap(s.kind()) }

// Lookback is the default window for a first run with no last_run_at.
func (s Schedule) Lookback() time.Duration {
	if s.Frequency == "weekly" {
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}
