package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database. Exercises the new SQL (API keys + tool queries) for real, including
// workspace isolation.
func TestToolsAndAPIKeysDB(t *testing.T) {
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
		w, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprint("org_", n), Name: "mcp test", Slug: fmt.Sprint("mcp-", n)})
		if err != nil {
			t.Fatal(err)
		}
		return w.ID
	}
	wsA, wsB := newWS(), newWS()

	t.Run("api key lifecycle", func(t *testing.T) {
		k, _ := apikey.Generate()
		row, err := q.CreateWorkspaceAPIKey(ctx, database.CreateWorkspaceAPIKeyParams{WorkspaceID: wsA, Name: "ci", Prefix: k.Display, KeyHash: k.Hash, Scopes: []string{"read"}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := q.GetActiveWorkspaceAPIKeyByHash(ctx, k.Hash)
		if err != nil || got.WorkspaceID != wsA {
			t.Fatalf("lookup: %v %+v", err, got)
		}
		if err := q.TouchWorkspaceAPIKey(ctx, database.TouchWorkspaceAPIKeyParams{ID: row.ID, WorkspaceID: wsA}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.RevokeWorkspaceAPIKey(ctx, database.RevokeWorkspaceAPIKeyParams{ID: row.ID, WorkspaceID: wsB}); err == nil {
			t.Fatal("another workspace must not revoke this key")
		}
		if _, err := q.RevokeWorkspaceAPIKey(ctx, database.RevokeWorkspaceAPIKeyParams{ID: row.ID, WorkspaceID: wsA}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.GetActiveWorkspaceAPIKeyByHash(ctx, k.Hash); err == nil {
			t.Fatal("revoked key must not authenticate")
		}
		list, _ := q.ListWorkspaceAPIKeys(ctx, wsA)
		if len(list) != 1 || !list[0].RevokedAt.Valid || !list[0].LastUsedAt.Valid {
			t.Fatalf("list %+v", list)
		}
		if other, _ := q.ListWorkspaceAPIKeys(ctx, wsB); len(other) != 0 {
			t.Fatal("list leaked across workspaces")
		}
	})

	reg := New(Deps{Q: q})
	run := func(ws, name, args string) map[string]any {
		t.Helper()
		out, err := reg.Call(ctx, ws, []string{"read", "write"}, name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, _ := json.Marshal(out)
		var m map[string]any
		json.Unmarshal(b, &m)
		return m
	}

	t.Run("listener create and tune", func(t *testing.T) {
		out := run(wsA, "create_listener", `{"name":"Buyers","looks_for":"asks for a CRM","keywords":["crm alternative","hubspot alternative"]}`)
		l := out["listener"].(map[string]any)
		id := l["id"].(string)
		if l["looks_for"] != "asks for a CRM" || len(out["keywords"].([]any)) != 2 {
			t.Fatalf("create: %v", out)
		}
		out = run(wsA, "tune_listener", `{"listener_id":"`+id+`","skips":"job posts","add_keywords":["crm alternative","pipedrive alternative"],"remove_keywords":["HubSpot Alternative"]}`)
		l = out["listener"].(map[string]any)
		if l["looks_for"] != "asks for a CRM" || l["skips"] != "job posts" || out["removed_keywords"].(float64) != 1 || len(out["skipped_keywords"].([]any)) != 1 {
			t.Fatalf("tune: %v", out)
		}
		if _, err := reg.Call(ctx, wsB, []string{"write"}, "tune_listener", json.RawMessage(`{"listener_id":"`+id+`","skips":"x"}`)); err == nil {
			t.Fatal("another workspace must not tune this listener")
		}
	})

	t.Run("reports run on real SQL", func(t *testing.T) {
		run(wsA, "attribution_report", `{"days":30}`)
		out := run(wsA, "weekly_summary", `{}`)
		if out["stats"].(map[string]any)["mentions"].(float64) != 0 {
			t.Fatalf("fresh workspace has no mentions: %v", out)
		}
		run(wsA, "search_mentions", `{"query":"crm"}`)
		run(wsA, "list_leads", `{"stage":"qualified"}`)
	})
}
