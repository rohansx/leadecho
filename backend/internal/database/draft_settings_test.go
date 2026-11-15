package database

import (
	"strings"
	"testing"
)

func TestAutoDraftLimit(t *testing.T) {
	n := func(v int) *int { return &v }
	for _, c := range []struct {
		name string
		in   *int
		want int
	}{
		{"unset: default", nil, DefaultAutoDraftDaily},
		{"off", n(0), 0},
		{"custom", n(3), 3},
		{"stored above the cap", n(500), MaxAutoDraftDaily},
		{"stored negative", n(-2), 0},
	} {
		if got := (DraftSettings{AutoDraftDaily: c.in}).AutoDraftLimit(); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestDraftSettingsClean(t *testing.T) {
	n := func(v int) *int { return &v }
	got, err := DraftSettings{ProofPoint: "  Acme cut\n interview hours  by a third ", SignOff: "\nRohan\nFounder, Utkrusht\n"}.Clean()
	if err != nil || got.ProofPoint != "Acme cut interview hours by a third" || got.SignOff != "Rohan\nFounder, Utkrusht" {
		t.Fatalf("clean: %+v %v", got, err)
	}
	for name, s := range map[string]DraftSettings{
		"long proof point": {ProofPoint: strings.Repeat("a", 401)},
		"long sign-off":    {SignOff: strings.Repeat("a", 201)},
		"too many a day":   {AutoDraftDaily: n(21)},
		"negative":         {AutoDraftDaily: n(-1)},
	} {
		if _, err := s.Clean(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestNextAutoDraftCount(t *testing.T) {
	for _, c := range []struct {
		name         string
		in           autoDraftCount
		delta, limit int
		want         autoDraftCount
		ok           bool
	}{
		{"first of the day", autoDraftCount{}, 1, 5, autoDraftCount{Day: "2026-11-15", N: 1}, true},
		{"yesterday's count does not carry over", autoDraftCount{Day: "2026-11-14", N: 5}, 1, 5, autoDraftCount{Day: "2026-11-15", N: 1}, true},
		{"last one", autoDraftCount{Day: "2026-11-15", N: 4}, 1, 5, autoDraftCount{Day: "2026-11-15", N: 5}, true},
		{"used up", autoDraftCount{Day: "2026-11-15", N: 5}, 1, 5, autoDraftCount{Day: "2026-11-15", N: 5}, false},
		{"switched off", autoDraftCount{}, 1, 0, autoDraftCount{Day: "2026-11-15"}, false},
		{"given back", autoDraftCount{Day: "2026-11-15", N: 2}, -1, 0, autoDraftCount{Day: "2026-11-15", N: 1}, true},
		{"never below zero", autoDraftCount{Day: "2026-11-15"}, -1, 0, autoDraftCount{Day: "2026-11-15"}, true},
	} {
		got, ok := nextAutoDraftCount(c.in, "2026-11-15", c.delta, c.limit)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %+v %v, want %+v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
