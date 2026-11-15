package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

// DB-backed: where the API does not run the monitor, finishing onboarding asks
// for the workspace's scan once, and only for a workspace the daily company
// scan would run for.
func TestCompleteRequestsTheFirstScanDB(t *testing.T) {
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
	h := NewOnboardingHandler(q, nil, nil, zerolog.Nop())
	h.SetCredits(billing.New(q, true))
	h.SetSetupFacts(true, true)

	newWorkspace := func(set string) string {
		t.Helper()
		var ws string
		if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('fs-'||gen_random_uuid(), 'fs', 'fs-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws) })
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET `+set+` WHERE id = $1`, ws); err != nil {
			t.Fatal(err)
		}
		return ws
	}
	complete := func(ws string) string {
		t.Helper()
		body := `{"brand_profile":{"product":"Acme"},"listeners":[{"goal":"find_buyers","name":"Buyers","keywords":["crm for agencies"],"platforms":["hackernews","reddit"]}]}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/onboarding/complete", strings.NewReader(body))
		h.Complete(rec, req.WithContext(middleware.WithClaims(ctx, &auth.Claims{WorkspaceID: ws})))
		if rec.Code != http.StatusOK {
			t.Fatalf("complete: %d %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	request := func(ws string) (raw string, scan monitor.ManualScan) {
		t.Helper()
		b, err := q.GetWorkspaceSettingsKey(ctx, ws, database.ManualScanKey)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &scan); err != nil {
				t.Fatal(err)
			}
		}
		return string(b), scan
	}

	t.Run("paid data and credits: asked for once", func(t *testing.T) {
		ws := newWorkspace(`paid_data = true, credit_balance = 100`)
		complete(ws)
		first, scan := request(ws)
		if scan.Kind != monitor.ScanPosts || scan.State != monitor.ScanRequested || scan.By != monitor.ScanByOnboarding {
			t.Fatalf("request = %s", first)
		}
		if body := complete(ws); !strings.Contains(body, "already_completed") {
			t.Fatalf("second complete: %s", body)
		}
		if again, _ := request(ws); again != first {
			t.Fatalf("a second Complete changed the request:\n%s\n%s", first, again)
		}
		// Asking again while it waits is refused by the store itself.
		h.requestFirstScan(ctx, ws)
		if again, _ := request(ws); again != first {
			t.Fatalf("a second request replaced the first:\n%s\n%s", first, again)
		}

		// The status: asked for, nothing searched yet, email owed to nobody (no user).
		rec := httptest.NewRecorder()
		h.SetupStatus(rec, httptest.NewRequest(http.MethodGet, "/onboarding/setup-status", nil).WithContext(middleware.WithClaims(ctx, &auth.Claims{WorkspaceID: ws})))
		var s setupStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || rec.Code != 200 {
			t.Fatalf("status: %d %s", rec.Code, rec.Body)
		}
		if len(s.Stages) != 4 || s.Stages[0].State != stageWaiting || s.Stages[0].Reason != "queued" || s.Stages[3].Reason != "queued" || s.HasLead || s.FinishedEmpty || s.EmailWhenReady {
			t.Fatalf("status = %s", rec.Body)
		}
	})

	for name, set := range map[string]string{
		"no paid data (free plan)": `paid_data = false, credit_balance = 100`,
		"out of credits":           `paid_data = true, credit_balance = 0`,
		"scanning paused":          `paid_data = true, credit_balance = 100, scanning_paused = true`,
	} {
		t.Run(name+": not asked for", func(t *testing.T) {
			ws := newWorkspace(set)
			complete(ws)
			if raw, _ := request(ws); raw != "" && raw != "null" {
				t.Fatalf("a scan was requested: %s", raw)
			}
		})
	}
}
