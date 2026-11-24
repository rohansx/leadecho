package platform

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// FeatureIdleAfter is how long a hidden page's background job keeps running
// after someone last opened the page. Demand and competitor watch spend
// model calls and paid data every day; a workspace that stopped looking at
// them should stop paying for them. Opening the page resumes the job.
//
// A new workspace gets the same window from its creation, so onboarding
// (competitor setup, the first demand themes) works before anyone has
// found the pages.
const FeatureIdleAfter = 14 * 24 * time.Hour

// FeatureInUse reports whether a feature still gets its background job at
// now: the workspace was created, or the feature last used, within
// FeatureIdleAfter. A zero lastUsed means never used.
func FeatureInUse(lastUsed, created, now time.Time) bool {
	latest := lastUsed
	if created.After(latest) {
		latest = created
	}
	return !latest.IsZero() && now.Sub(latest) <= FeatureIdleAfter
}

// featureLookup returns when a feature was last used and when the workspace
// was created (*database.Queries.FeatureLastUsed).
type featureLookup func(ctx context.Context, ws, feature string) (lastUsed, created time.Time, err error)

// featureActive applies FeatureInUse to a looked-up workspace. A failed
// lookup keeps the job running: skipping a user's job is worse than one
// extra run.
func featureActive(ctx context.Context, lookup featureLookup, logger zerolog.Logger, ws, feature string, now time.Time) bool {
	last, created, err := lookup(ctx, ws, feature)
	if err != nil {
		logger.Warn().Err(err).Str("workspace_id", ws).Str("feature", feature).Msg("feature use: lookup failed")
		return true
	}
	return FeatureInUse(last, created, now)
}

func (s *Shared) featureInUse(ctx context.Context, ws, feature string) bool {
	return featureActive(ctx, s.Queries.FeatureLastUsed, s.Logger, ws, feature, time.Now())
}
