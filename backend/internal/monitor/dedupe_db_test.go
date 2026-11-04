package monitor

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: the insert path drops bots, alternate URL forms of the same
// post, and near-identical text; distinct posts still insert.
func TestInsertMentionDedupeDB(t *testing.T) {
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
	var wsID string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('dd-'||gen_random_uuid(), 'dd', 'dd-'||gen_random_uuid()) RETURNING id`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, wsID)

	m := &Monitor{q: database.New(pool), logger: zerolog.Nop()}
	text := "Looking for a good alternative to Mailchimp for our small ecommerce store, pricing went up again and the editor is slow"
	mk := func(pid, url, author, content string) database.CreateMentionParams {
		return database.CreateMentionParams{
			WorkspaceID: wsID, Platform: "reddit", PlatformID: pid, Url: url, Content: content,
			AuthorUsername:   pgtype.Text{String: author, Valid: true},
			PlatformMetadata: []byte("{}"), EngagementMetrics: []byte("{}"), KeywordMatches: []string{},
		}
	}
	tests := []struct {
		name string
		p    database.CreateMentionParams
		want bool
	}{
		{"first insert", mk("a1", "https://www.reddit.com/r/x/comments/a1/t/", "alice", text), true},
		{"old.reddit + utm form of same URL", mk("a1-old", "https://old.reddit.com/r/x/comments/a1/t?utm_source=share", "alice", "different words entirely here"), false},
		{"near-identical text, other URL", mk("a2", "https://reddit.com/r/y/comments/a2", "bob", text+"!!"), false},
		{"bot author", mk("a3", "https://reddit.com/r/x/comments/a3", "AutoModerator", "Your post was removed for breaking rule 2 of this community, please read the rules"), false},
		{"distinct post", mk("a4", "https://reddit.com/r/x/comments/a4", "carol", "We are hiring a senior Go engineer in Berlin to work on our payments platform remote friendly"), true},
	}
	for _, tt := range tests {
		if got := m.insertMention(ctx, tt.p, "kw") != nil; got != tt.want {
			t.Errorf("%s: inserted = %v, want %v", tt.name, got, tt.want)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mentions WHERE workspace_id = $1 AND canonical_url IS NOT NULL AND content_simhash IS NOT NULL`, wsID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows with dedupe keys = %d (%v), want 2", n, err)
	}
}
