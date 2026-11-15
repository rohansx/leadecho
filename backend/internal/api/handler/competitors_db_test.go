package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/competitors"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestCompetitorsAPIDB(t *testing.T) {
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

	const secret = "test-secret"
	workspace := func(name string) (string, string) {
		slug := fmt.Sprintf("cwapi-%s-%d", name, time.Now().UnixNano())
		ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: slug, Name: name, Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID) })
		token, err := auth.IssueToken(secret, "00000000-0000-0000-0000-0000000000aa", ws.ID, "t@leadecho.test", "T", "owner", 0)
		if err != nil {
			t.Fatal(err)
		}
		return ws.ID, token
	}
	wsA, tokenA := workspace("a")
	_, tokenB := workspace("b")

	home := `<html><body>` + strings.Repeat("Hiring tests for engineering teams. ", 10) + `<a href="/pricing">Pricing</a></body></html>`
	svc := &competitors.Service{
		Q: q, Logger: zerolog.Nop(),
		Fetch: func(_ context.Context, u string) (string, error) { return home, nil },
		Limit: func(context.Context, string) int { return 2 },
		Search: func(_ context.Context, name string) ([]monitor.ExaWebResult, error) {
			if name == "Findable" {
				return []monitor.ExaWebResult{{Title: "Findable", URL: "https://findable.io/"}}, nil
			}
			return nil, nil
		},
	}
	h := NewCompetitorsHandler(svc, zerolog.Nop())
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret, nil))
	r.Get("/competitors", h.List)
	r.Post("/competitors", h.Create)
	r.Patch("/competitors/{id}", h.Update)
	r.Delete("/competitors/{id}", h.Delete)
	r.Get("/competitors/{id}/people", h.People)
	r.Post("/competitors/{id}/sources", h.AddSource)
	r.Delete("/competitors/{id}/sources/{sourceID}", h.DeleteSource)
	srv := httptest.NewServer(r)
	defer srv.Close()

	call := func(token, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		req.AddCookie(&http.Cookie{Name: "session", Value: token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// Validation.
	for name, body := range map[string]any{
		"no name":     map[string]string{"website": "https://acme.com"},
		"bad website": map[string]string{"name": "Acme", "website": "ftp://acme.com"},
	} {
		if code, _ := call(tokenA, "POST", "/competitors", body); code != 400 {
			t.Errorf("%s: want 400, got %d", name, code)
		}
	}
	if code, _ := call(tokenA, "POST", "/competitors", map[string]string{"name": "Nowhere Inc"}); code != 422 {
		t.Errorf("a name with no findable website is refused, got %d", code)
	}

	// A name alone is resolved; with a website it is used as given.
	code, out := call(tokenA, "POST", "/competitors", map[string]string{"name": "Findable"})
	if code != 201 || out["data"].(map[string]any)["website"] != "https://findable.io" {
		t.Fatalf("create by name: %d %v", code, out)
	}
	code, out = call(tokenA, "POST", "/competitors", map[string]string{"name": "Acme", "website": "acme.com"})
	if code != 201 {
		t.Fatalf("create: %d %v", code, out)
	}
	acme := out["data"].(map[string]any)["id"].(string)
	if code, _ := call(tokenA, "POST", "/competitors", map[string]string{"name": "Third", "website": "third.com"}); code != 409 {
		t.Errorf("past the limit: want 409, got %d", code)
	}

	// The view carries the discovered sources.
	code, out = call(tokenA, "GET", "/competitors", nil)
	list := out["data"].([]any)
	if code != 200 || len(list) != 2 || out["limit"].(float64) != 2 {
		t.Fatalf("list: %d %v", code, out)
	}
	var sources []any
	for _, c := range list {
		if c.(map[string]any)["id"] == acme {
			sources = c.(map[string]any)["sources"].([]any)
		}
	}
	if len(sources) != 2 {
		t.Fatalf("want landing + pricing, got %v", sources)
	}

	// Sources by hand: validated and normalised.
	if code, _ := call(tokenA, "POST", "/competitors/"+acme+"/sources", map[string]string{"kind": "x", "url": "https://x.com/intent/tweet"}); code != 400 {
		t.Errorf("bad x source: want 400, got %d", code)
	}
	code, out = call(tokenA, "POST", "/competitors/"+acme+"/sources", map[string]string{"kind": "x", "url": "@Acme"})
	if code != 201 || out["data"].(map[string]any)["url"] != "https://x.com/acme" {
		t.Fatalf("add source: %d %v", code, out)
	}

	// Another workspace sees nothing and can touch nothing.
	if code, out := call(tokenB, "GET", "/competitors", nil); code != 200 || len(out["data"].([]any)) != 0 {
		t.Fatalf("workspace B list: %d %v", code, out)
	}
	for _, c := range []struct{ method, path string }{
		{"PATCH", "/competitors/" + acme}, {"DELETE", "/competitors/" + acme},
		{"GET", "/competitors/" + acme + "/people"}, {"POST", "/competitors/" + acme + "/sources"},
		{"DELETE", "/competitors/not-a-uuid"},
	} {
		body := map[string]string{"status": "paused", "kind": "blog", "url": "https://acme.com/blog"}
		if code, _ := call(tokenB, c.method, c.path, body); code != 404 {
			t.Errorf("workspace B %s %s: want 404, got %d", c.method, c.path, code)
		}
	}

	// Pause, people (none), delete.
	if code, out := call(tokenA, "PATCH", "/competitors/"+acme, map[string]string{"status": "paused"}); code != 200 || out["data"].(map[string]any)["status"] != "paused" {
		t.Fatalf("pause: %d %v", code, out)
	}
	if code, _ := call(tokenA, "PATCH", "/competitors/"+acme, map[string]string{"status": "gone"}); code != 400 {
		t.Errorf("bad status: want 400, got %d", code)
	}
	if code, out := call(tokenA, "GET", "/competitors/"+acme+"/people", nil); code != 200 || len(out["data"].([]any)) != 0 {
		t.Fatalf("people: %d %v", code, out)
	}
	if code, _ := call(tokenA, "DELETE", "/competitors/"+acme, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	var left int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM competitor_sources WHERE workspace_id = $1 AND competitor_id = $2", wsA, acme).Scan(&left)
	if left != 0 {
		t.Fatalf("deleting a competitor removes its sources, %d left", left)
	}
}
