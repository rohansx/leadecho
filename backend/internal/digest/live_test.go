//go:build live

// Live digest send through Resend. Sends ONE real email. Run:
//
//	LEADECHO_TEST_DATABASE_URL=... RESEND_API_KEY=... LIVE_DIGEST_TO=you@example.com \
//	LIVE_DIGEST_FROM='LeadEcho <onboarding@resend.dev>' go test -tags live -run TestDigestLive -v ./internal/digest/
package digest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

func TestDigestLive(t *testing.T) {
	dsn, key, to, from := os.Getenv("LEADECHO_TEST_DATABASE_URL"), os.Getenv("RESEND_API_KEY"), os.Getenv("LIVE_DIGEST_TO"), os.Getenv("LIVE_DIGEST_FROM")
	if dsn == "" || key == "" || to == "" || from == "" {
		t.Skip("needs LEADECHO_TEST_DATABASE_URL, RESEND_API_KEY, LIVE_DIGEST_TO, LIVE_DIGEST_FROM")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)

	now := time.Now().UTC()
	wsID, _ := setupWorkspace(t, ctx, pool, q, "live")
	seedPeriod(t, ctx, pool, wsID, now.Add(-2*time.Hour))

	svc := NewService(q, fakeSummarizer{text: "Live test: 2 buyers, 1 noisy keyword."}, key, from, "https://leadecho.app", zerolog.New(os.Stderr))
	sched := DefaultSchedule()
	sched.Timezone = "UTC"
	sched.DailyTime = now.Add(-time.Minute).Format("15:04")
	sched.Recipients = []string{to}
	sched.Channels = []string{"email"}
	if _, err := svc.SaveSettings(ctx, wsID, sched); err != nil {
		t.Fatal(err)
	}

	// Only this workspace may send: switch every other workspace's digests off,
	// so seed/demo owners never receive a real email.
	rows, err := pool.Query(ctx, `SELECT id::text FROM workspaces WHERE id <> $1`, wsID)
	if err != nil {
		t.Fatal(err)
	}
	others, err := pgxCollectStrings(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range others {
		if _, err := svc.SaveSettings(ctx, id, Schedule{DailyTime: "09:30", WeeklyDay: "monday", Timezone: "UTC", Channels: []string{}}); err != nil {
			t.Fatalf("disable digests for %s: %v", id, err)
		}
	}

	svc.RunDue(ctx)

	var status, msg string
	if err := pool.QueryRow(ctx, `SELECT status::text, error FROM digest_runs WHERE workspace_id = $1 ORDER BY created_at DESC LIMIT 1`, wsID).Scan(&status, &msg); err != nil {
		t.Fatalf("no digest run recorded: %v", err)
	}
	t.Logf("digest run status=%s msg=%q", status, msg)
	if status != "sent" {
		t.Fatalf("want sent, got %s (%s)", status, msg)
	}
}

func pgxCollectStrings(rows pgx.Rows) ([]string, error) {
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
