package billing

// Daily prospecting (companies found in the customer segments, each with a
// confirmed buyer, research and a first message) is a Pro feature. It is on
// for a Pro workspace unless an admin switched it off; the switch is stored
// under workspaces.settings.prospect_daily. An operator's manual scan from
// /ops ignores it.

const (
	ProspectDailyKey  = "prospect_daily"
	ProspectDailyPlan = "pro"
)

// ProspectDaily reports whether the daily prospect scan runs for a workspace
// on plan, given its stored switch (nil when never set).
func ProspectDaily(plan string, stored []byte) bool {
	return plan == ProspectDailyPlan && string(stored) != "false"
}
