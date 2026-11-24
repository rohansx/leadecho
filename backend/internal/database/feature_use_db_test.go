package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestFeatureUseDB(t *testing.T) {
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
	q := New(pool)
	slug := fmt.Sprintf("fu-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, CreateWorkspaceParams{ClerkOrgID: slug, Name: "Feature use test", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID) //nolint:errcheck

	if at, created, err := q.FeatureLastUsed(ctx, ws.ID, FeatureDemand); err != nil || !at.IsZero() || time.Since(created) > time.Minute {
		t.Fatalf("never used: got %v (created %v), %v", at, created, err)
	}
	if err := q.MarkFeatureUsed(ctx, ws.ID, FeatureDemand); err != nil {
		t.Fatal(err)
	}
	first, _, err := q.FeatureLastUsed(ctx, ws.ID, FeatureDemand)
	if err != nil || time.Since(first) > time.Minute {
		t.Fatalf("after mark: got %v, %v", first, err)
	}
	if other, _, _ := q.FeatureLastUsed(ctx, ws.ID, FeatureCompetitors); !other.IsZero() {
		t.Fatal("marking demand must not mark competitors")
	}
	// Within the hour a second mark writes nothing.
	time.Sleep(10 * time.Millisecond)
	if err := q.MarkFeatureUsed(ctx, ws.ID, FeatureDemand); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := q.FeatureLastUsed(ctx, ws.ID, FeatureDemand); !again.Equal(first) {
		t.Fatalf("second mark within the hour rewrote the time: %v -> %v", first, again)
	}
	// Other settings keys survive.
	if ok, err := q.ClaimSettingsRun(ctx, ws.ID, "demand", time.Hour); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if at, _, _ := q.FeatureLastUsed(ctx, ws.ID, FeatureDemand); !at.Equal(first) {
		t.Fatal("another settings key wiped feature_use")
	}
}
