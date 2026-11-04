// Package digest builds and sends the daily/weekly LeadEcho digest (P1 #13).
package digest

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	_ "time/tzdata" // timezone names must resolve in slim containers
)

type Kind string

const (
	KindDaily  Kind = "daily"
	KindWeekly Kind = "weekly"
)

// DueWindow is how long after its scheduled time a digest may still go out.
// A worker that was down longer than this skips that period instead of
// sending a stale digest.
const DueWindow = 6 * time.Hour

const maxRecipients = 10

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
	"wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday,
	"saturday": time.Saturday,
}

var validChannels = map[string]bool{"email": true, "slack": true}

// Schedule is a workspace's digest preference.
type Schedule struct {
	DailyEnabled  bool     `json:"daily_enabled"`
	DailyTime     string   `json:"daily_time"` // HH:MM, local to Timezone
	WeeklyEnabled bool     `json:"weekly_enabled"`
	WeeklyDay     string   `json:"weekly_day"` // lowercase English weekday
	Timezone      string   `json:"timezone"`   // IANA name
	Recipients    []string `json:"recipients"` // empty = workspace owner
	Channels      []string `json:"channels"`   // email | slack
}

func DefaultSchedule() Schedule {
	return Schedule{
		DailyEnabled: true, DailyTime: "09:30",
		WeeklyEnabled: true, WeeklyDay: "monday",
		Timezone: "UTC", Recipients: []string{}, Channels: []string{"email"},
	}
}

// Period is the window a digest covers; End is the scheduled send time.
type Period struct {
	Start time.Time
	End   time.Time
}

// Normalize validates the schedule and returns a cleaned copy.
func (s Schedule) Normalize() (Schedule, error) {
	out := s
	hh, mm, err := parseClock(s.DailyTime)
	if err != nil {
		return Schedule{}, err
	}
	out.DailyTime = fmt.Sprintf("%02d:%02d", hh, mm)

	out.WeeklyDay = strings.ToLower(strings.TrimSpace(s.WeeklyDay))
	if _, ok := weekdays[out.WeeklyDay]; !ok {
		return Schedule{}, errors.New("weekly_day must be a weekday name (e.g. monday)")
	}

	out.Timezone = strings.TrimSpace(s.Timezone)
	if out.Timezone == "" {
		out.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(out.Timezone); err != nil {
		return Schedule{}, fmt.Errorf("unknown timezone %q", out.Timezone)
	}

	if out.Recipients, err = normalizeRecipients(s.Recipients); err != nil {
		return Schedule{}, err
	}
	if out.Channels, err = normalizeChannels(s.Channels); err != nil {
		return Schedule{}, err
	}
	return out, nil
}

func parseClock(v string) (int, int, error) {
	var hh, mm int
	v = strings.TrimSpace(v)
	if n, err := fmt.Sscanf(v, "%d:%d", &hh, &mm); err != nil || n != 2 || hh < 0 || hh > 23 || mm < 0 || mm > 59 || strings.Count(v, ":") != 1 {
		return 0, 0, errors.New("daily_time must be HH:MM (24h)")
	}
	return hh, mm, nil
}

func normalizeRecipients(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, r := range in {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" || seen[r] {
			continue
		}
		addr, err := mail.ParseAddress(r)
		if err != nil || addr.Address != r {
			return nil, fmt.Errorf("invalid recipient email %q", r)
		}
		seen[r] = true
		out = append(out, r)
	}
	if len(out) > maxRecipients {
		return nil, fmt.Errorf("at most %d recipients", maxRecipients)
	}
	return out, nil
}

func normalizeChannels(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if !validChannels[c] {
			return nil, fmt.Errorf("invalid channel %q (email or slack)", c)
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// HasChannel reports whether the user opted into a delivery channel.
func (s Schedule) HasChannel(c string) bool {
	for _, v := range s.Channels {
		if v == c {
			return true
		}
	}
	return false
}

// Due returns the most recent scheduled period for kind at or before now and
// whether it should be sent now (enabled and within DueWindow). Computation
// happens on the local wall clock, so DST shifts keep the send time stable.
func (s Schedule) Due(kind Kind, now time.Time) (Period, bool) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.UTC
	}
	hh, mm, err := parseClock(s.DailyTime)
	if err != nil {
		hh, mm = 9, 30
	}
	t := now.In(loc)
	enabled := s.DailyEnabled
	days := 1
	back := 0
	if kind == KindWeekly {
		enabled, days = s.WeeklyEnabled, 7
		target, ok := weekdays[s.WeeklyDay]
		if !ok {
			target = time.Monday
		}
		back = (int(t.Weekday()) - int(target) + 7) % 7
	}
	occ := time.Date(t.Year(), t.Month(), t.Day()-back, hh, mm, 0, 0, loc)
	if occ.After(t) {
		occ = occ.AddDate(0, 0, -days)
	}
	p := Period{Start: occ.AddDate(0, 0, -days).UTC(), End: occ.UTC()}
	return p, enabled && now.Sub(occ) < DueWindow
}

// TrailingPeriod is the window ending now, used for previews.
func TrailingPeriod(kind Kind, now time.Time) Period {
	days := 1
	if kind == KindWeekly {
		days = 7
	}
	return Period{Start: now.AddDate(0, 0, -days).UTC(), End: now.UTC()}
}
