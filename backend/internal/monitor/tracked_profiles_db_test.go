package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// DB-backed: a due tracked LinkedIn profile is polled through (fake) treg,
// its new post becomes a mention, and it is not due again until the interval.
func TestTrackedProfilesPollDB(t *testing.T) {
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
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('tp-'||gen_random_uuid(), 'tp', 'tp-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	var tools []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tools = append(tools, strings.TrimPrefix(r.URL.Path, "/call/"))
		_, _ = w.Write([]byte(`{"rows":[{"text":"We just raised and are hiring SDRs","url":"https://www.linkedin.com/feed/update/urn:li:activity:7514893923710984192/","posted_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}]}`))
	}))
	defer srv.Close()
	m := &Monitor{q: q, logger: zerolog.Nop(), treg: treg.New(treg.Config{Token: "t", BaseURL: srv.URL}, nil, zerolog.Nop()), trackedEvery: time.Hour}

	p, err := q.CreateTrackedProfile(ctx, database.CreateTrackedProfileParams{WorkspaceID: ws, Platform: "linkedin", Handle: "acmeceo", Url: "https://www.linkedin.com/in/acmeceo"})
	if err != nil {
		t.Fatal(err)
	}
	due := func() bool {
		rows, err := q.ListDueTrackedProfiles(ctx, database.ListDueTrackedProfilesParams{Before: time.Now().Add(-time.Hour), MaxRows: 1000})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.ID == p.ID {
				return true
			}
		}
		return false
	}
	if !due() {
		t.Fatal("new profile should be due")
	}
	alerts := m.pollTrackedProfile(ctx, p)
	if len(alerts) != 1 || alerts[0].Platform != "linkedin" || tools[0] != tregLinkedInUserPostsTool {
		t.Fatalf("alerts=%+v tools=%v", alerts, tools)
	}
	if again := m.pollTrackedProfile(ctx, p); len(again) != 0 {
		t.Fatalf("same post inserted twice: %+v", again)
	}
	if err := q.MarkTrackedProfilePolled(ctx, database.MarkTrackedProfilePolledParams{ID: p.ID, WorkspaceID: ws}); err != nil {
		t.Fatal(err)
	}
	if due() {
		t.Fatal("just-polled profile should not be due")
	}
}
