package monitor

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: a brand keyword's post that does not name the brand is never
// stored; the yield counts what each keyword stored; a keyword with many
// posts and no lead on a platform is polled once a day there, and its first
// lead lifts that.
func TestBrandKeywordAndYieldDB(t *testing.T) {
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
	var ws, prof string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('ky-'||gen_random_uuid(), 'ky', 'ky-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if err := pool.QueryRow(ctx, `INSERT INTO monitoring_profiles (workspace_id, name, description) VALUES ($1, 'p', 'd') RETURNING id`, ws).Scan(&prof); err != nil {
		t.Fatal(err)
	}
	keyword := func(term string) database.ListActiveKeywordsRow {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO keywords (workspace_id, profile_id, term, platforms) VALUES ($1, $2, $3, '{hackernews,bluesky}') RETURNING id`, ws, prof, term).Scan(&id); err != nil {
			t.Fatalf("keyword %q: %v", term, err)
		}
		return database.ListActiveKeywordsRow{ID: id, WorkspaceID: ws, Term: term, Platforms: []string{"hackernews", "bluesky"}, IsActive: true}
	}
	brandKW, noisy := keyword("Utkrusht interview"), keyword("coding test")

	m := &Monitor{q: database.New(pool), logger: zerolog.Nop()}
	bctx := withScanBrand(ctx, newBrand("Utkrusht", "https://utkrusht.ai"))
	post := func(kw database.ListActiveKeywordsRow, pid, content string) database.CreateMentionParams {
		return database.CreateMentionParams{
			WorkspaceID: ws, KeywordID: pgUUID(kw.ID), Platform: "hackernews", PlatformID: pid, Url: "https://news.example/" + pid,
			Content: content, AuthorUsername: pgtype.Text{String: "someone", Valid: true},
			PlatformMetadata: []byte("{}"), EngagementMetrics: []byte("{}"), KeywordMatches: []string{kw.Term},
		}
	}
	if a := m.insertMention(bctx, post(brandKW, "b1", "My onsite interview at a large retailer went badly and I am not sure what to study next"), brandKW.Term); a != nil {
		t.Error("a brand keyword's post that does not name the brand was stored")
	}
	if a := m.insertMention(bctx, post(brandKW, "b2", "We ran our last backend hiring round through utkrusht.ai and the shortlist was better than our take-home"), brandKW.Term); a == nil {
		t.Error("a post that names the brand was dropped")
	}
	if a := m.insertMention(bctx, post(noisy, "n0", "Does anyone still give a coding test before the first call, or is it all pairing sessions now?"), noisy.Term); a == nil {
		t.Error("a non-brand keyword's post was dropped")
	}

	// "hiring signal" is a label, not a customer keyword: a product called
	// Signal must not lose its hiring leads to the brand check.
	label := post(noisy, "h1", "A fintech company posted a job for a senior backend engineer in Berlin this week")
	label.KeywordID = pgtype.UUID{}
	if a := m.insertMention(withScanBrand(ctx, newBrand("Signal", "")), label, "hiring signal"); a == nil {
		t.Error("a post without a customer keyword went through the brand check")
	}
	// Posts stored for the brand keyword before this check existed count for nothing.
	if _, err := pool.Exec(ctx, `
INSERT INTO mentions (workspace_id, keyword_id, platform, platform_id, url, content, scoring_metadata, platform_metadata, engagement_metrics, keyword_matches)
SELECT $1, $2, 'hackernews', 'legacy'||g, 'https://news.example/legacy'||g, 'an interview story '||g, '{"stage":"stage0_brand_not_named"}', '{}', '{}', '{}'
FROM generate_series(1, 200) g`, ws, brandKW.ID); err != nil {
		t.Fatal(err)
	}

	// 120 more posts for the noisy keyword on Hacker News, none of them a lead.
	if _, err := pool.Exec(ctx, `
INSERT INTO mentions (workspace_id, keyword_id, platform, platform_id, url, content, scoring_metadata, relevance_score, platform_metadata, engagement_metrics, keyword_matches)
SELECT $1, $2, 'hackernews', 'bulk'||g, 'https://news.example/bulk'||g, 'agent chatter '||g,
       CASE WHEN g <= 100 THEN '{"stage":"stage1_spam_filter"}' WHEN g <= 115 THEN '{"stage":"stage3_classified","ask":"A need"}' ELSE '{"stage":"stage2_low_similarity"}' END::jsonb,
       CASE WHEN g BETWEEN 101 AND 115 THEN 3 END, '{}', '{}', '{}'
FROM generate_series(1, 120) g`, ws, noisy.ID); err != nil {
		t.Fatal(err)
	}

	rows := m.keywordYield(ctx, ws)
	by := map[string]database.KeywordYield{}
	for _, r := range rows {
		by[r.Term+"/"+r.Platform] = r
	}
	if got := by["Utkrusht interview/hackernews"]; got.Posts != 1 {
		t.Errorf("brand keyword yield = %+v, want the one real mention and nothing else", got)
	}
	if got := by["coding test/hackernews"]; got.Posts != 121 || got.Passed != 20 || got.Leads != 0 || got.Asks != 15 {
		t.Errorf("noisy keyword yield = %+v, want 121 posts, 20 past the first filter, 0 leads, 15 asks", got)
	}

	// Scanned 5 hours ago on both platforms: Hacker News is throttled to once
	// a day, Bluesky (no posts) keeps its hourly cadence.
	scanned := func() {
		for _, p := range []string{"hackernews", "bluesky"} {
			if _, err := pool.Exec(ctx, `INSERT INTO keyword_platform_scans (keyword_id, workspace_id, platform, last_scanned_at) VALUES ($1, $2, $3, NOW() - INTERVAL '5 hours')
ON CONFLICT (keyword_id, platform) DO UPDATE SET last_scanned_at = EXCLUDED.last_scanned_at`, noisy.ID, ws, p); err != nil {
				t.Fatal(err)
			}
		}
	}
	scanned()
	due := func() string {
		for _, kw := range m.applyCadence(ctx, ws, []database.ListActiveKeywordsRow{noisy}) {
			return fmt.Sprint(kw.Platforms)
		}
		return "[]"
	}
	if got := due(); got != "[bluesky]" {
		t.Errorf("due platforms = %s, want [bluesky]: Hacker News brought 121 posts and no lead", got)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT cadence::text FROM keywords WHERE id = $1`, noisy.ID).Scan(&stored); err != nil || stored != "{}" {
		t.Errorf("the customer's cadence was changed: %q (%v)", stored, err)
	}

	// The first lead lifts the throttle.
	if _, err := pool.Exec(ctx, `UPDATE mentions SET relevance_score = 7 WHERE workspace_id = $1 AND platform_id = 'bulk101'`, ws); err != nil {
		t.Fatal(err)
	}
	if got := due(); got != "[hackernews bluesky]" {
		t.Errorf("due platforms after a lead = %s, want [hackernews bluesky]", got)
	}
}
