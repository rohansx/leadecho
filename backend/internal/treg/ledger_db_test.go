package treg

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database. Covers the ledger write path and per-workspace spend isolation.
func TestLedgerSpendDB(t *testing.T) {
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

	newWS := func() string {
		var id string
		slug := fmt.Sprintf("treg-%d", time.Now().UnixNano())
		if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ($1, $1, $1) RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, id) })
		return id
	}
	wsA, wsB := newWS(), newWS()

	srv := newCostServer(t)
	c := New(Config{Token: "tok", BaseURL: srv}, q, zerolog.Nop())
	for _, ws := range []string{wsA, wsA, wsB} {
		if _, err := c.Call(ctx, Request{WorkspaceID: ws, Tool: "treg.x.search.posts"}); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Now().Add(-time.Hour)
	rows, err := q.TregSpendByTool(ctx, database.TregSpendByToolParams{WorkspaceID: wsA, Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CostMicro != 1500 || rows[0].Calls != 2 {
		t.Fatalf("wsA rows = %+v", rows)
	}
	rows, _ = q.TregSpendByTool(ctx, database.TregSpendByToolParams{WorkspaceID: wsB, Since: since})
	if len(rows) != 1 || rows[0].Calls != 1 {
		t.Fatalf("wsB rows = %+v (must not see wsA)", rows)
	}
	rows, _ = q.TregSpendByTool(ctx, database.TregSpendByToolParams{WorkspaceID: wsA, Since: time.Now().Add(time.Hour)})
	if len(rows) != 0 {
		t.Fatalf("since filter ignored: %+v", rows)
	}
}

func newCostServer(t *testing.T) string {
	t.Helper()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Treg-Cost-Micro", "750")
		w.Header().Set("X-Treg-Call-Id", "call_"+r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{}`))
	})
	return c.cfg.BaseURL
}
