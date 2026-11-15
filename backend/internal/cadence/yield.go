package cadence

import "time"

// What a keyword brought in, and what follows from it. Pure logic; no DB.

const (
	// YieldWindow is how far back a keyword's results are counted.
	YieldWindow = 14 * 24 * time.Hour
	// ThrottleMinPosts: a keyword on one platform that stored this many posts
	// in the window without a single lead is polled once a day and filtered on
	// the cheapest model, until it produces a lead.
	ThrottleMinPosts = 100
	// LowYieldMinPosts: a keyword that stored this many posts across its
	// platforms without a lead is flagged to the customer, who may pause it.
	LowYieldMinPosts = 150
	throttledCadence = "24h"
)

// Yield counts a keyword's posts in the window: stored, kept by the first
// (noise) filter, scored as leads, and stating a need.
type Yield struct {
	Posts  int `json:"posts"`
	Passed int `json:"passed"`
	Leads  int `json:"leads"`
	Asks   int `json:"asks"`
}

func (y Yield) Add(o Yield) Yield {
	return Yield{Posts: y.Posts + o.Posts, Passed: y.Passed + o.Passed, Leads: y.Leads + o.Leads, Asks: y.Asks + o.Asks}
}

// Throttled is for one keyword on one platform.
func (y Yield) Throttled() bool { return y.Posts >= ThrottleMinPosts && y.Leads == 0 }

// LowYield is for a keyword across its platforms.
func (y Yield) LowYield() bool { return y.Posts >= LowYieldMinPosts && y.Leads == 0 }

// Throttle returns the cadence with the given platforms polled at most once
// a day. The customer's stored cadence (m) is not changed.
func (m Map) Throttle(platforms []string) Map {
	out := make(Map, len(m)+len(platforms))
	for p, v := range m {
		out[p] = v
	}
	for _, p := range platforms {
		out[p] = throttledCadence
	}
	return out
}
