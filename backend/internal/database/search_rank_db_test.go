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
func TestSearchRankQueriesDB(t *testing.T) {
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
	slug := fmt.Sprintf("sr-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, CreateWorkspaceParams{ClerkOrgID: slug, Name: "Search rank test", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID) //nolint:errcheck

	if th, err := q.ListSearchRankThreads(ctx, ws.ID, 10); err != nil || len(th) != 0 {
		t.Fatalf("threads = %v %v", th, err)
	}
	ids, err := q.ListWorkspacesDueSearchRank(ctx, time.Now(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == ws.ID {
			t.Fatal("a workspace with no lead threads is never due")
		}
	}
	// The claim is atomic: the first caller wins, a second within the week loses.
	if won, err := q.ClaimSettingsRun(ctx, ws.ID, SearchRankKey, 7*24*time.Hour); err != nil || !won {
		t.Fatalf("first claim = %v %v", won, err)
	}
	if won, err := q.ClaimSettingsRun(ctx, ws.ID, SearchRankKey, 7*24*time.Hour); err != nil || won {
		t.Fatalf("second claim = %v %v, want lost", won, err)
	}
	// A mention that does not exist is not an error, set or cleared.
	if err := q.SetMentionSearchRank(ctx, ws.ID, ws.ID, []byte(`{"query":"q","position":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := q.SetMentionSearchRank(ctx, ws.ID, ws.ID, nil); err != nil {
		t.Fatal(err)
	}
}
