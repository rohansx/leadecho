package mutes

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/database"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database. Covers the mutes + keyword cadence/scan queries (migration 00032).
func TestSourcesQueriesDB(t *testing.T) {
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
		n := time.Now().UnixNano()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ($1, 'mutes test', $1) RETURNING id`, fmt.Sprint("mutes-", n)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, id) })
		return id
	}
	ws1, ws2 := newWS(), newWS()

	t.Run("mute add is idempotent, case-insensitive and workspace scoped", func(t *testing.T) {
		s := NewStore(q)
		if err := s.Add(ctx, ws1, KindAuthor, "Spammer"); err != nil {
			t.Fatal(err)
		}
		if err := s.Add(ctx, ws1, KindAuthor, "@SPAMMER"); err != nil {
			t.Fatal(err)
		}
		list, err := s.List(ctx, ws1)
		if err != nil || len(list) != 1 {
			t.Fatalf("list = %v, %v; want 1 row", list, err)
		}
		if muted, _ := s.IsMuted(ctx, ws1, Candidate{Author: "spammer"}); !muted {
			t.Error("want muted in ws1")
		}
		if muted, _ := s.IsMuted(ctx, ws2, Candidate{Author: "spammer"}); muted {
			t.Error("mute leaked into ws2")
		}
		if ok, _ := s.Delete(ctx, ws2, list[0].ID); ok {
			t.Error("cross-workspace delete succeeded")
		}
		if ok, err := s.Delete(ctx, ws1, list[0].ID); !ok || err != nil {
			t.Errorf("delete = %v, %v", ok, err)
		}
		if muted, _ := s.IsMuted(ctx, ws1, Candidate{Author: "spammer"}); muted {
			t.Error("still muted after delete")
		}
	})

	t.Run("bluesky platform, cadence and scans", func(t *testing.T) {
		var kw struct{ ID string }
		if err := pool.QueryRow(ctx, `WITH p AS (INSERT INTO monitoring_profiles (workspace_id, name) VALUES ($1, 'p') RETURNING id)
			 INSERT INTO keywords (workspace_id, profile_id, term, platforms) SELECT $1, p.id, 'crm', '{bluesky,twitter}' FROM p RETURNING id`, ws1).Scan(&kw.ID); err != nil {
			t.Fatalf("create keyword with bluesky: %v", err)
		}
		if _, err := q.UpdateKeywordCadence(ctx, database.UpdateKeywordCadenceParams{Cadence: []byte(`{"twitter":"6h"}`), ID: kw.ID, WorkspaceID: ws2}); err == nil {
			t.Error("cross-workspace cadence update succeeded")
		}
		if _, err := q.UpdateKeywordCadence(ctx, database.UpdateKeywordCadenceParams{Cadence: []byte(`{"twitter":"6h"}`), ID: kw.ID, WorkspaceID: ws1}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ { // upsert twice
			if err := q.UpsertKeywordPlatformScan(ctx, database.UpsertKeywordPlatformScanParams{Platform: "twitter", KeywordID: kw.ID, WorkspaceID: ws1}); err != nil {
				t.Fatal(err)
			}
		}
		// Wrong workspace: no row is written.
		_ = q.UpsertKeywordPlatformScan(ctx, database.UpsertKeywordPlatformScanParams{Platform: "bluesky", KeywordID: kw.ID, WorkspaceID: ws2})
		scans, err := q.ListKeywordPlatformScans(ctx, ws1)
		if err != nil || len(scans) != 1 || scans[0].Platform != "twitter" {
			t.Fatalf("scans = %+v, %v", scans, err)
		}
		cads, err := q.ListKeywordCadences(ctx, ws1)
		if err != nil || len(cads) != 1 || string(cads[0].Cadence) != `{"twitter": "6h"}` {
			t.Fatalf("cadences = %+v (%s), %v", cads, cads[0].Cadence, err)
		}
	})
}
