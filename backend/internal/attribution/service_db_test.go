package attribution

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
)

// DB-backed: a repeated event_id is a duplicate and rolls back with its
// counter; a link's revenue total can pass int32.
func TestRecordConversionDedupeDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)
	s := NewService(q, zerolog.Nop(), crmsync.NewDispatcher(q, make([]byte, 32), zerolog.Nop()))

	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('at-'||gen_random_uuid(), 'at', 'at-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	link, err := q.CreateUTMLink(ctx, database.CreateUTMLinkParams{WorkspaceID: ws, Code: "at" + ws[:8], DestinationUrl: "https://x.co", UtmSource: "leadecho", UtmMedium: "social_reply"})
	if err != nil {
		t.Fatal(err)
	}

	big := ConversionRequest{UTMCode: link.Code, EventType: "purchase", RevenueCents: 2_000_000_000, Metadata: map[string]any{"event_id": "evt_1"}}
	if _, err := s.RecordConversion(ctx, ws, big); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordConversion(ctx, ws, big); !errors.Is(err, ErrDuplicateConversion) {
		t.Fatalf("repeat event_id: err = %v, want duplicate", err)
	}
	big.Metadata = map[string]any{"event_id": "evt_2"}
	got, err := s.RecordConversion(ctx, ws, big)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignupCount != 2 || got.RevenueCents != 4_000_000_000 {
		t.Fatalf("signups=%d revenue=%d, want 2 and 4e9 (duplicate rolled back, no int32 overflow)", got.SignupCount, got.RevenueCents)
	}
	// No event_id: never deduplicated.
	for range 2 {
		if _, err := s.RecordConversion(ctx, ws, ConversionRequest{UTMCode: link.Code}); err != nil {
			t.Fatal(err)
		}
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM utm_events WHERE utm_link_id = $1`, link.ID).Scan(&events); err != nil || events != 4 {
		t.Fatalf("events = %d (%v), want 4", events, err)
	}
}
