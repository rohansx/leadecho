package engagers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/researcher"
	"leadecho/internal/treg"
)

// DB-backed: engagers become linkedin mentions + leads tagged engager with an
// ICP score; pulling the same post again updates instead of duplicating.
func TestImporterDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('eg-'||gen_random_uuid(), 'eg', 'eg-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/call/") {
		case ToolComments:
			_, _ = w.Write([]byte(commentsBody))
		case ToolReactions:
			_, _ = w.Write([]byte(reactionsBody))
		}
	}))
	defer srv.Close()
	tc := treg.New(treg.Config{Token: "t", BaseURL: srv.URL}, nil, zerolog.Nop())
	imp := NewImporter(q, tc, researcher.NewService(q, zerolog.Nop()), zerolog.Nop())

	const postURL = "https://www.linkedin.com/feed/update/urn:li:activity:7508596155145043968/"
	ref, _ := ParsePostRef(postURL)
	req := Request{PostURL: postURL, Ref: ref, Kinds: []string{KindComments, KindReactions}}
	for run := 0; run < 2; run++ {
		rows, err := imp.Pull(ctx, ws, req)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("run %d: rows = %d", run, len(rows))
		}
		for _, r := range rows {
			if r.LeadID == "" || r.MentionID == "" || r.ICPFitScore == nil {
				t.Fatalf("row %+v", r)
			}
		}
		if rows[0].Company != "Northwind" || *rows[0].ICPFitScore <= *rows[2].ICPFitScore {
			t.Errorf("commenter with company should outscore a bare reactor: %v vs %v", *rows[0].ICPFitScore, *rows[2].ICPFitScore)
		}
	}
	var leads, mentions, unclassified int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM leads WHERE workspace_id = $1 AND 'engager' = ANY(tags) AND metadata->>'source' = 'engager'`, ws).Scan(&leads)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM mentions WHERE workspace_id = $1 AND platform = 'linkedin' AND platform_metadata->>'source' = 'engager'`, ws).Scan(&mentions)
	un, _ := q.ListUnclassifiedMentions(ctx, database.ListUnclassifiedMentionsParams{WorkspaceID: ws, Lim: 10})
	unclassified = len(un)
	if leads != 3 || mentions != 3 || unclassified != 0 {
		t.Fatalf("leads=%d mentions=%d unclassified=%d", leads, mentions, unclassified)
	}
}
