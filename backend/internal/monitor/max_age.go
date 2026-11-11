package monitor

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// maxMentionAge is how far back any scan reaches. Search APIs without a date
// filter (Bluesky, Lobsters, Indie Hackers, Google News, Exa) return their
// newest matches, which for a rare keyword can be months old.
const maxMentionAge = 7 * 24 * time.Hour

// tooOld reports whether a post predates the scan window: the backfill window
// from WithSince when set, else maxMentionAge. Posts without a date are kept.
func tooOld(ctx context.Context, created pgtype.Timestamptz, now time.Time) bool {
	if !created.Valid {
		return false
	}
	cutoff := now.Add(-maxMentionAge)
	if since := sinceFrom(ctx); !since.IsZero() {
		cutoff = since
	}
	return created.Time.Before(cutoff)
}
