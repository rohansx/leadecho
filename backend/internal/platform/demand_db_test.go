package platform

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/demand"
)

// askStore records which workspaces the scheduler looked at; it has no asks,
// so nothing is grouped and no model is called.
type askStore struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *askStore) LoadState(context.Context, string) (demand.State, error) {
	return demand.State{}, nil
}
func (s *askStore) UpdateState(_ context.Context, _ string, fn func(demand.State) (demand.State, error)) (demand.State, error) {
	return fn(demand.State{})
}
func (s *askStore) RecentAsks(_ context.Context, ws string) ([]demand.Ask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[ws] = true
	return nil, nil
}
func (s *askStore) Product(context.Context, string) (demand.Product, error) {
	return demand.Product{}, nil
}
func (s *askStore) AskSignals(context.Context, string, []string) (map[string]demand.Signal, error) {
	return nil, nil
}
func (s *askStore) CreateDraft(context.Context, string, string, string) error { return nil }

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestDemandSchedulerSkipsIdleWorkspacesDB(t *testing.T) {
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

	// newWS makes a workspace with an active keyword, created daysOld ago.
	newWS := func(tag string, daysOld int) string {
		n := time.Now().UnixNano()
		w, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprintf("org_fu_%s_%d", tag, n), Name: "Focus " + tag, Slug: fmt.Sprintf("fu-%s-%d", tag, n)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", w.ID) }) //nolint:errcheck
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET created_at = now() - make_interval(days => $2) WHERE id = $1`, w.ID, daysOld); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `WITH p AS (INSERT INTO monitoring_profiles (workspace_id, name) VALUES ($1, 'p') RETURNING id)
			INSERT INTO keywords (workspace_id, term, profile_id) SELECT $1, 'k', id FROM p`, w.ID); err != nil {
			t.Fatal(err)
		}
		return w.ID
	}
	fresh := newWS("fresh", 1)
	idle := newWS("idle", 30)
	opened := newWS("opened", 30)
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET settings = jsonb_build_object('feature_use', jsonb_build_object('demand', now() - interval '1 day')) WHERE id = $1`, opened); err != nil {
		t.Fatal(err)
	}

	store := &askStore{seen: map[string]bool{}}
	s := &Shared{Queries: q, Logger: zerolog.Nop()}
	s.runDemand(ctx, &demand.Service{Store: store, Credits: demand.NoCredits{}, Logger: zerolog.Nop()})

	for ws, want := range map[string]bool{fresh: true, idle: false, opened: true} {
		if store.seen[ws] != want {
			t.Errorf("workspace %s: checked=%v, want %v", ws, store.seen[ws], want)
		}
	}
}
