// Package billing: plans, prepaid credits and Dodo Payments.
//
// The credit balance lives in Postgres (workspaces.credit_balance) so every
// metered action is checked instantly; Dodo handles checkout, subscriptions
// and top-up purchases, and its webhooks add credits.
package billing

// Plan is one sellable tier. Credits are added on each paid renewal; the free
// plan is topped back up to its allowance every 30 days.
type Plan struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PriceCents     int    `json:"price_cents"`
	MonthlyCredits int    `json:"monthly_credits"`
	KeywordLimit   int    `json:"keyword_limit"`
}

// Plans in display order. Measured cost is ~$0.0035 per post scored on
// Sonnet 5.5 plus ~$0.001 per paid search, so these keep ~60% margin at
// full use.
var Plans = []Plan{
	{ID: "free", Name: "Free", PriceCents: 0, MonthlyCredits: 300, KeywordLimit: 3},
	{ID: "starter", Name: "Starter", PriceCents: 4900, MonthlyCredits: 5000, KeywordLimit: 15},
	{ID: "pro", Name: "Pro", PriceCents: 14900, MonthlyCredits: 20000, KeywordLimit: 50},
}

// Top-up pack: one-time purchase, credits never expire.
const (
	TopupCredits    = 1000
	TopupPriceCents = 1000
)

// Credit costs per metered action.
const (
	CostPost   = 1  // one post sent to the AI (filter + classify)
	CostSearch = 1  // one paid search through treg
	CostDraft  = 5  // one draft reply
	CostChat   = 2  // one chat question
	CostEnrich = 10 // one lead enrichment (several paid lookups)
	// CostResearch is one company researched on the web for a confirmed lead
	// (several searches read by the strongest model). Refunded if nothing
	// with a source is found.
	CostResearch = 15
)

// Reasons label spending and grants in the usage breakdown.
const (
	ReasonPost     = "post_scored"
	ReasonSearch   = "search"
	ReasonDraft    = "draft_reply"
	ReasonChat     = "chat"
	ReasonEnrich   = "enrichment"
	ReasonResearch = "research"
	ReasonPlan     = "plan_allowance"
	ReasonTopup    = "topup"
	ReasonAdjust   = "adjustment"
)

// PlanByID returns the plan with that id.
func PlanByID(id string) (Plan, bool) {
	for _, p := range Plans {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}
