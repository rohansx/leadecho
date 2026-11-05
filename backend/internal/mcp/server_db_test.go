package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/apikey"
	"leadecho/internal/database"
	"leadecho/internal/tools"
)

// DB-backed end to end: a real key through APIKeyAuth into the MCP server.
func TestMCPWithAPIKeyDB(t *testing.T) {
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
	n := time.Now().UnixNano()
	w, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprint("org_", n), Name: "mcp e2e", Slug: fmt.Sprint("mcp-e2e-", n)})
	if err != nil {
		t.Fatal(err)
	}
	k, _ := apikey.Generate()
	if _, err := q.CreateWorkspaceAPIKey(ctx, database.CreateWorkspaceAPIKeyParams{WorkspaceID: w.ID, Name: "e2e", Prefix: k.Display, KeyHash: k.Hash, Scopes: []string{"read"}}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Registry: tools.New(tools.Deps{Q: q}), Version: "t", Logger: zerolog.Nop(), Auth: middleware.APIKeyPrincipal}
	h := middleware.APIKeyAuth(q, nil, 0)(srv)

	do := func(token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := do(k.Plaintext, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"weekly_summary","arguments":{}}}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"isError":false`) || !strings.Contains(rec.Body.String(), "Last 7 days") {
		t.Fatalf("weekly_summary: %d %s", rec.Code, rec.Body)
	}
	rec = do(k.Plaintext, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_stage","arguments":{}}}`)
	if !strings.Contains(rec.Body.String(), "requires the write scope") {
		t.Fatalf("read key must not write: %s", rec.Body)
	}
	other, _ := apikey.Generate()
	if rec := do(other.Plaintext, `{"jsonrpc":"2.0","id":3,"method":"ping"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key: %d", rec.Code)
	}
}
