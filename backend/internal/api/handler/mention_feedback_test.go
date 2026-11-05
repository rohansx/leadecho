package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

func TestParseFeedbackBody(t *testing.T) {
	tests := []struct {
		name, body string
		wantErr    string
		wantLabel  string
	}{
		{"up", `{"label":"up"}`, "", "up"},
		{"down with note", `{"label":"DOWN","note":" job post "}`, "", "down"},
		{"bad label", `{"label":"meh"}`, "label must be", ""},
		{"missing label", `{}`, "label must be", ""},
		{"long note", `{"label":"up","note":"` + strings.Repeat("x", 501) + `"}`, "note must be", ""},
		{"bad json", `{`, "invalid JSON", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFeedbackBody(strings.NewReader(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got.Label != tt.wantLabel {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestMentionListRejectsUnknownTagAndSentiment(t *testing.T) {
	h := NewMentionHandler(nil, nil) // validation must fail before any query
	for _, qs := range []string{"tag=buy_signal", "tag=nonsense", "sentiment=angry"} {
		rec := httptest.NewRecorder()
		h.List(rec, httptest.NewRequest(http.MethodGet, "/mentions?"+qs, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", qs, rec.Code)
		}
	}
}

func TestFeedbackRejectsBadIDAndBody(t *testing.T) {
	h := NewMentionHandler(nil, nil)
	tests := []struct{ id, body string }{
		{"not-a-uuid", `{"label":"up"}`},
		{"11111111-1111-1111-1111-111111111111", `{"label":"sideways"}`},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.Feedback(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)), "id", tt.id))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status %d, want 400", tt.id, tt.body, rec.Code)
		}
	}
}

func withURLParam(r *http.Request, k, v string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(k, v)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

// DB-backed: feedback → mention_feedback → few-shot examples for the listener,
// plus tag/sentiment filtering and the dedupe lookup.
func TestMentionFeedbackDB(t *testing.T) {
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

	var wsID, profID, kwID, mentionID string
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('fb-'||gen_random_uuid(), 'fb', 'fb-'||gen_random_uuid()) RETURNING id`), &wsID)
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, wsID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO monitoring_profiles (workspace_id, name, description, looks_for, skips) VALUES ($1, 'Uptime', 'monitoring', 'teams leaving Pingdom', 'job posts') RETURNING id`, wsID), &profID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO keywords (workspace_id, term, platforms, profile_id) VALUES ($1, 'pingdom', '{reddit}', $2) RETURNING id`, wsID, profID), &kwID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO mentions (workspace_id, keyword_id, platform, platform_id, url, title, content, intent_tags, sentiment, canonical_url, created_at)
		VALUES ($1, $2, 'reddit', 'p1', 'https://reddit.com/r/x/1', 'Pingdom too pricey', 'need cheaper alerts', '{buying_intent,pain_point}', 'negative', 'https://reddit.com/r/x/1', now()) RETURNING id`, wsID, kwID), &mentionID)

	h := NewMentionHandler(q, nil)
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := withURLParam(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), "id", mentionID)
		h.feedbackFor(rec, req, wsID, "")
		return rec
	}
	if rec := post(`{"label":"down","note":"meh"}`); rec.Code != http.StatusOK {
		t.Fatalf("feedback status %d: %s", rec.Code, rec.Body)
	}
	rec := post(`{"label":"up","note":"perfect lead"}`) // same user: replaces the vote
	if rec.Code != http.StatusOK {
		t.Fatalf("feedback status %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Feedback struct{ Label, ProfileID string } `json:"feedback"`
		Mention  MentionResponse                   `json:"mention"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.Contains(string(resp.Mention.ScoringMetadata), `"label":"up"`) {
		t.Fatalf("scoring_metadata not updated: %s", resp.Mention.ScoringMetadata)
	}

	sc := monitor.ScoringContextFor(ctx, q, nil, wsID, mentionID, func(err error) { t.Fatal(err) })
	if sc.LooksFor != "teams leaving Pingdom" || sc.Skips != "job posts" {
		t.Fatalf("listener intent missing: %+v", sc)
	}
	if len(sc.Examples) != 1 || sc.Examples[0].Label != "up" || sc.Examples[0].Note != "perfect lead" {
		t.Fatalf("examples = %+v", sc.Examples)
	}

	rows, err := q.ListMentionsComposed(ctx, database.ListMentionsComposedParams{WorkspaceID: wsID, Tag: "pain_point", Sentiment: "negative", Lim: 10})
	if err != nil || len(rows) != 1 || len(rows[0].IntentTags) != 2 {
		t.Fatalf("tag filter: %v rows=%d", err, len(rows))
	}
	if rows, _ := q.ListMentionsComposed(ctx, database.ListMentionsComposedParams{WorkspaceID: wsID, Tag: "hiring", Lim: 10}); len(rows) != 0 {
		t.Fatalf("tag filter should exclude, got %d", len(rows))
	}

	dup, err := q.FindRecentDuplicateMention(ctx, database.FindRecentDuplicateMentionParams{
		WorkspaceID: wsID, LookbackDays: 7, CanonicalUrl: "https://reddit.com/r/x/1", MaxDistance: -1,
	})
	if err != nil || !dup {
		t.Fatalf("canonical dup = %v, %v", dup, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mentions SET content_simhash = $1 WHERE id = $2`, int64(0b1011), mentionID); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		hash int64
		dist int32
		want bool
	}{{0b1010, 1, true}, {0b0100, 3, false}, {0b0100, 4, true}, {0b1011, -1, false}} {
		got, err := q.FindRecentDuplicateMention(ctx, database.FindRecentDuplicateMentionParams{
			WorkspaceID: wsID, LookbackDays: 7, CanonicalUrl: "https://other", Simhash: tt.hash, MaxDistance: tt.dist,
		})
		if err != nil || got != tt.want {
			t.Errorf("simhash %b within %d: got %v (%v), want %v", tt.hash, tt.dist, got, err, tt.want)
		}
	}
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dst *string) {
	t.Helper()
	if err := row.Scan(dst); err != nil {
		t.Fatal(err)
	}
}
