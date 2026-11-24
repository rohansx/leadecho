package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/database"
)

// DB-backed: a second lead for the same post is a 409 with a clear message,
// not a 500 (leads_mention_id_unique).
func TestCreateLeadDuplicateMentionDB(t *testing.T) {
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
	var ws, mention string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('ld-'||gen_random_uuid(), 'ld', 'ld-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if err := pool.QueryRow(ctx, `INSERT INTO mentions (workspace_id, platform, platform_id, url, content, engagement_metrics, keyword_matches)
VALUES ($1, 'reddit', 'dup', 'https://x/dup', 'a post', '{}', '{}') RETURNING id::text`, ws).Scan(&mention); err != nil {
		t.Fatal(err)
	}

	h := NewLeadHandler(database.New(pool), nil)
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/leads", strings.NewReader(`{"mention_id":"`+mention+`"}`))
		req = req.WithContext(middleware.WithClaims(req.Context(), &auth.Claims{WorkspaceID: ws}))
		rec := httptest.NewRecorder()
		h.Create(rec, req)
		return rec
	}
	if rec := post(); rec.Code != http.StatusCreated {
		t.Fatalf("first lead: %d %s", rec.Code, rec.Body)
	}
	if rec := post(); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already a lead") {
		t.Fatalf("second lead: %d %s, want 409", rec.Code, rec.Body)
	}
}
