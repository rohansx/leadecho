// Package cadence holds per-platform scan cadence for keywords (stored in
// keywords.cadence as {"twitter":"1h","reddit":"30m"}) and the monthly data
// cost estimate derived from it. Pure logic; no DB.
package cadence

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Default applies to free platforms without an explicit cadence.
const Default = time.Hour

// PaidDefault applies to paid platforms (searched through treg). Each search
// costs the customer a credit, and searches cover the past week, so twice a
// day finds the same posts as every 4h for a third of the credits: at 4h a
// plan's own keyword limit cost more credits than the plan includes
// (Starter: 15 keywords x 3 platforms x 6/day x 30 = 8,100 of 5,000).
const PaidDefault = 12 * time.Hour

// defaultFor is the cadence a platform gets when none is set.
func defaultFor(platform string) time.Duration {
	if CostPerCall[platform] > 0 {
		return PaidDefault
	}
	return Default
}

func defaultLabel(platform string) string {
	if CostPerCall[platform] > 0 {
		return "12h"
	}
	return "1h"
}

// skew lets a scan that finished slightly less than one interval ago count as
// due, so a cadence equal to the tick interval doesn't slip every other tick.
const skew = time.Minute

// Options are the cadences the UI offers; anything else is rejected.
var Options = map[string]time.Duration{
	"15m": 15 * time.Minute, "30m": 30 * time.Minute, "1h": time.Hour,
	"3h": 3 * time.Hour, "4h": 4 * time.Hour, "6h": 6 * time.Hour, "12h": 12 * time.Hour, "24h": 24 * time.Hour,
}

// CostPerCall is the data cost (USD) of one search call. Platforms not listed
// are unknown and rejected by Parse. Keep in sync with the platform_type enum.
var CostPerCall = map[string]float64{
	"twitter":    0.00075,
	"linkedin":   0.002,
	"reddit":     0.0009, // treg anyapi.reddit.search (observed $0.0004-0.0012)
	"hackernews": 0, "devto": 0, "bluesky": 0,
	"lobsters": 0, "indiehackers": 0, "quora": 0, "exa": 0, "googlenews": 0,
}

// Map is platform -> cadence label.
type Map map[string]string

// Parse decodes and validates a stored/submitted cadence object.
func Parse(raw []byte) (Map, error) {
	m := Map{}
	if len(raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cadence must be an object of platform to interval: %w", err)
	}
	for p, v := range m {
		if _, ok := CostPerCall[p]; !ok {
			return nil, fmt.Errorf("unknown platform %q", p)
		}
		if _, ok := Options[v]; !ok {
			return nil, fmt.Errorf("unsupported cadence %q for %s (use 15m, 30m, 1h, 3h, 4h, 6h, 12h or 24h)", v, p)
		}
	}
	return m, nil
}

// Interval returns the scan interval for platform (its default if unset/invalid).
func (m Map) Interval(platform string) time.Duration {
	if d, ok := Options[m[platform]]; ok {
		return d
	}
	return defaultFor(platform)
}

func (m Map) label(platform string) string {
	if _, ok := Options[m[platform]]; ok {
		return m[platform]
	}
	return defaultLabel(platform)
}

// Due returns the platforms (in input order) whose last scan is at least one
// interval old, or which were never scanned.
func Due(platforms []string, m Map, last map[string]time.Time, now time.Time) []string {
	due := make([]string, 0, len(platforms))
	for _, p := range platforms {
		t, ok := last[p]
		if !ok || now.Sub(t) >= m.Interval(p)-skew {
			due = append(due, p)
		}
	}
	return due
}

// PlatformCost is the monthly estimate for one keyword on one platform.
type PlatformCost struct {
	Platform      string  `json:"platform"`
	Cadence       string  `json:"cadence"`
	CallsPerMonth int     `json:"calls_per_month"`
	USDPerMonth   float64 `json:"usd_per_month"`
}

// KeywordCost totals a keyword's platforms.
type KeywordCost struct {
	Platforms   []PlatformCost `json:"platforms"`
	USDPerMonth float64        `json:"usd_per_month"`
}

const month = 30 * 24 * time.Hour

// Preview estimates calls and USD per month, assuming one search call per
// keyword per platform per scan.
func Preview(platforms []string, m Map) KeywordCost {
	out := KeywordCost{Platforms: make([]PlatformCost, 0, len(platforms))}
	for _, p := range platforms {
		calls := int(month / m.Interval(p))
		usd := round4(float64(calls) * CostPerCall[p])
		out.Platforms = append(out.Platforms, PlatformCost{Platform: p, Cadence: m.label(p), CallsPerMonth: calls, USDPerMonth: usd})
		out.USDPerMonth += usd
	}
	out.USDPerMonth = round4(out.USDPerMonth)
	return out
}

func round4(f float64) float64 { return math.Round(f*1e4) / 1e4 }
