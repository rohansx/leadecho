package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// 00047 adds brand_profiles.archetype (checked, default ”) and the down
// migration drops it. Runs against LEADECHO_TEST_DATABASE_URL (migrated) and
// leaves it migrated.
func TestArchetypeMigrationDB(t *testing.T) {
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
	slug := fmt.Sprintf("arch-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, CreateWorkspaceParams{ClerkOrgID: slug, Name: "Archetype test", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID)

	upsert := func(archetype, suggested *string) string {
		t.Helper()
		txt := func(s *string) pgtype.Text {
			if s == nil {
				return pgtype.Text{}
			}
			return pgtype.Text{String: *s, Valid: true}
		}
		bp, err := q.UpsertBrandProfile(ctx, UpsertBrandProfileParams{WorkspaceID: ws.ID, Archetype: txt(archetype), SuggestedArchetype: txt(suggested),
			CustomerSegments: []byte("[]"), PainPoints: []byte("[]"), Keywords: []byte("{}"), Competitors: []string{}, Features: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		return bp.Archetype
	}
	s := func(v string) *string { return &v }
	for i, c := range []struct {
		archetype, suggested *string
		want                 string
	}{
		{nil, nil, ""},                         // insert, nothing given
		{nil, s("hiring"), "hiring"},           // a suggestion fills an empty one
		{nil, s("devtool"), "hiring"},          // ...but never replaces one
		{nil, nil, "hiring"},                   // omitted keeps
		{s("devtool"), s("hiring"), "devtool"}, // explicit wins
		{s(""), nil, ""},                       // explicit empty clears
		{s("devtool"), nil, "devtool"},
	} {
		if got := upsert(c.archetype, c.suggested); got != c.want {
			t.Errorf("step %d: archetype %q, want %q", i, got, c.want)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE brand_profiles SET archetype = 'agency' WHERE workspace_id = $1`, ws.ID); err == nil {
		t.Error("an unknown archetype must violate the check")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(db, "../../migrations", 46); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'brand_profiles' AND column_name = 'archetype'`).Scan(&n)
	if err := goose.Up(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("down migration left the archetype column")
	}
	if got, err := q.GetBrandProfile(ctx, ws.ID); err != nil || got.Archetype != "" {
		t.Errorf("after down and up: archetype %q (want default ''), err %v", got.Archetype, err)
	}
}
