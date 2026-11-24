package platform

import (
	"context"
	"time"
)

// FeatureIdleAfter is how long a hidden page's background job keeps running
// after someone last opened the page. Demand and competitor watch spend
// model calls and paid data every day; a workspace that stopped looking at
// them should stop paying for them. Opening the page resumes the job.
const FeatureIdleAfter = 14 * 24 * time.Hour

// FeatureInUse reports whether a feature last used at lastUsed still gets
// its background job at now. Never used (zero) means no.
func FeatureInUse(lastUsed, now time.Time) bool {
	return !lastUsed.IsZero() && now.Sub(lastUsed) <= FeatureIdleAfter
}

// featureInUse looks the feature up for a workspace. A failed lookup keeps
// the job running: skipping a user's job is worse than one extra run.
func (s *Shared) featureInUse(ctx context.Context, ws, feature string) bool {
	last, err := s.Queries.FeatureLastUsed(ctx, ws, feature)
	if err != nil {
		s.Logger.Warn().Err(err).Str("workspace_id", ws).Str("feature", feature).Msg("feature use: lookup failed")
		return true
	}
	return FeatureInUse(last, time.Now())
}
