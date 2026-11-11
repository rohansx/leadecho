package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestTooOld(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(-d), Valid: true} }
	backfill := WithSince(context.Background(), now.Add(-2*24*time.Hour))

	for _, tc := range []struct {
		name    string
		ctx     context.Context
		created pgtype.Timestamptz
		want    bool
	}{
		{"fresh post", context.Background(), at(time.Hour), false},
		{"just inside 7 days", context.Background(), at(7*24*time.Hour - time.Minute), false},
		{"85 days old", context.Background(), at(85 * 24 * time.Hour), true},
		{"no date is kept", context.Background(), pgtype.Timestamptz{}, false},
		{"backfill window wins", backfill, at(3 * 24 * time.Hour), true},
		{"inside backfill window", backfill, at(24 * time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tooOld(tc.ctx, tc.created, now); got != tc.want {
				t.Fatalf("tooOld = %v, want %v", got, tc.want)
			}
		})
	}
}
