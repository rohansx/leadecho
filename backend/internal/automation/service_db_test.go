package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/tools"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database. Covers the real tools registry, per-period claims, lead sweep,
// noise archiving, keyword value and tenant isolation.
func TestAutomationsDB(t *testing.T) {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	newWS := func(tag string) string {
		n := time.Now().UnixNano()
		w, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprintf("org_auto_%s_%d", tag, n), Name: "Auto " + tag, Slug: fmt.Sprintf("auto-%s-%d", tag, n)})
		if err != nil {
			t.Fatal(err)
		}
		return w.ID
	}
	wsID, otherWS := newWS("a"), newWS("b")
	at := time.Now().Add(-2 * time.Hour)
	var kwID string
	if err := pool.QueryRow(ctx, `WITH p AS (INSERT INTO monitoring_profiles (workspace_id, name) VALUES ($1, 'p') RETURNING id)
		INSERT INTO keywords (workspace_id, term, profile_id) SELECT $1, 'noisy', id FROM p RETURNING id`, wsID).Scan(&kwID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		exec(`INSERT INTO mentions (workspace_id, keyword_id, platform, platform_id, url, content, relevance_score, intent, created_at)
			VALUES ($1, $2, 'reddit', $3, $4, 'meh', 1.5, 'general', $5)`, wsID, kwID, fmt.Sprintf("n%d-%s", i, wsID), fmt.Sprintf("https://reddit.com/n/%d", i), at)
	}
	exec(`INSERT INTO mentions (workspace_id, platform, platform_id, url, title, content, relevance_score, intent, created_at)
		VALUES ($1, 'hackernews', $2, 'https://news.ycombinator.com/item?id=1', 'Need a tool for X', 'buying', 9.1, 'buy_signal', $3)`, wsID, "b1-"+wsID, at)
	exec(`INSERT INTO mentions (workspace_id, platform, platform_id, url, content, relevance_score, intent, created_at)
		VALUES ($1, 'reddit', $2, 'https://reddit.com/other', 'other tenant', 9.9, 'buy_signal', $3)`, otherWS, "o-"+otherWS, at)

	svc := NewService(q, tools.New(tools.Deps{Q: q}), "", "", "https://app.test", zerolog.Nop())
	create := func(kind, cfg string) database.Automation {
		a, err := q.CreateAutomation(ctx, database.CreateAutomationParams{WorkspaceID: wsID, Name: kind, Kind: kind,
			Frequency: "daily", RunTime: "09:30", Weekday: "monday", Timezone: "UTC", Enabled: true, Config: []byte(cfg)})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	t.Run("lead sweep creates one lead per buyer, scoped to the workspace", func(t *testing.T) {
		a := create(KindLeadSweep, `{}`)
		for i := 0; i < 2; i++ { // second run must not duplicate
			run, err := svc.RunNow(ctx, a)
			if err != nil || run.Status != "succeeded" {
				t.Fatalf("run %d = %+v err=%v", i, run, err)
			}
		}
		var n, other int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE workspace_id = $1 AND 'automation' = ANY(tags)`, wsID).Scan(&n)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE workspace_id = $1`, otherWS).Scan(&other)
		if n != 1 || other != 0 {
			t.Fatalf("leads = %d (other ws %d), want 1/0", n, other)
		}
		runs, err := q.ListAutomationRuns(ctx, database.ListAutomationRunsParams{AutomationID: a.ID, WorkspaceID: wsID, Lim: 10})
		if err != nil || len(runs) != 2 || runs[0].Trigger != "manual" {
			t.Fatalf("runs = %+v err=%v", runs, err)
		}
		if leak, _ := q.ListAutomationRuns(ctx, database.ListAutomationRunsParams{AutomationID: a.ID, WorkspaceID: otherWS, Lim: 10}); len(leak) != 0 {
			t.Fatal("runs leaked across workspaces")
		}
	})

	t.Run("triage archives noise when allowed; scheduled claim is once per period", func(t *testing.T) {
		a := create(KindDailyTriage, `{"mark_noise":true}`)
		slot, _ := ScheduleOf(a).Due(time.Now())
		svc.now = func() time.Time { return slot.Add(time.Minute) }
		defer func() { svc.now = time.Now }()
		// RunDue scans every workspace; only count this automation's runs.
		svc.RunDue(ctx)
		svc.RunDue(ctx)
		runs, _ := q.ListAutomationRuns(ctx, database.ListAutomationRunsParams{AutomationID: a.ID, WorkspaceID: wsID, Lim: 10})
		if len(runs) != 1 || runs[0].Trigger != "schedule" || runs[0].Status != "succeeded" {
			t.Fatalf("runs = %+v", runs)
		}
		var out Output
		_ = json.Unmarshal(runs[0].Output, &out)
		if len(out.Items) != 1 || out.Items[0].Title != "Need a tool for X" {
			t.Fatalf("output = %s", runs[0].Output)
		}
		var archived int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM mentions WHERE workspace_id = $1 AND status = 'archived'`, wsID).Scan(&archived)
		if archived != 6 {
			t.Fatalf("archived = %d, want 6", archived)
		}
		var notes int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE workspace_id = $1 AND channel = 'in_app' AND metadata->>'automation_id' = $2`, wsID, a.ID).Scan(&notes)
		if notes != 1 {
			t.Fatalf("in-app notifications = %d", notes)
		}
	})

	t.Run("attribution review suggests muting the noisy keyword", func(t *testing.T) {
		run, err := svc.RunNow(ctx, create(KindAttributionReview, `{}`))
		if err != nil || run.Status != "succeeded" {
			t.Fatalf("run = %+v err=%v", run, err)
		}
		var out Output
		_ = json.Unmarshal(run.Output, &out)
		if len(out.Suggestions) != 1 {
			t.Fatalf("output = %s", run.Output)
		}
	})
}
