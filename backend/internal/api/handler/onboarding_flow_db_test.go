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
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
	"leadecho/internal/onboarding"
)

type fakePublicSearch struct{}

func (fakePublicSearch) Search(_ context.Context, platform, term string, _ time.Time) ([]monitor.Post, error) {
	return []monitor.Post{{Platform: platform, URL: "https://news.ycombinator.com/item?id=" + strings.ReplaceAll(term, " ", "_"), Title: term, Content: "Looking for a good " + term + " for my agency team", CreatedAt: time.Now()}}, nil
}

type fakeBackfill struct {
	mu       sync.Mutex
	lookback time.Duration
}

func (f *fakeBackfill) ScanWorkspaceNow(string) {}
func (f *fakeBackfill) ScanWorkspaceSince(_ string, d time.Duration) {
	f.mu.Lock()
	f.lookback = d
	f.mu.Unlock()
}
func (f *fakeBackfill) IsScanning(string) bool { return true }

const siteHTML = `<html><head><title>Acme | CRM for agencies</title></head><body>
<nav><a href="/pricing">Pricing</a></nav>
<h1>Acme keeps agency deals moving.</h1><p>%s</p></body></html>`

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestOnboardingFlowDB(t *testing.T) {
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

	slug := fmt.Sprintf("onb-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: slug, Name: "Onboarding test", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID)

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, siteHTML, strings.Repeat("Pipelines, reminders and reports for client work. ", 5))
	}))
	defer site.Close()

	scanner := &fakeBackfill{}
	h := NewOnboardingHandler(q, nil, nil, zerolog.Nop())
	h.fetch = onboarding.PlainFetcher(site.Client())
	h.search = fakePublicSearch{}
	h.SetScanner(scanner)

	const secret = "test-secret"
	token, err := auth.IssueToken(secret, "00000000-0000-0000-0000-0000000000aa", ws.ID, "t@leadecho.test", "T", "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret, nil))
	r.Get("/brand-profile", h.GetBrandProfile)
	r.Put("/brand-profile", h.PutBrandProfile)
	r.Post("/onboarding/analyze", h.AnalyzeStream)
	r.Post("/onboarding/listeners/build", h.BuildListeners)
	r.Post("/onboarding/listeners/feedback", h.SampleFeedback)
	r.Post("/onboarding/complete", h.Complete)
	r.Get("/onboarding/first-value", h.FirstValue)
	srv := httptest.NewServer(r)
	defer srv.Close()

	call := func(method, path string, body any) (int, string) {
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
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	t.Run("brand profile missing before analysis", func(t *testing.T) {
		if code, _ := call("GET", "/brand-profile", nil); code != 404 {
			t.Fatalf("want 404, got %d", code)
		}
		if code, _ := call("POST", "/onboarding/listeners/build", map[string]any{"goals": []string{"find_buyers"}}); code != 400 {
			t.Fatalf("build before analysis: want 400, got %d", code)
		}
	})

	t.Run("analyze streams and persists profile + KB seed (keyless)", func(t *testing.T) {
		if code, _ := call("POST", "/onboarding/analyze", map[string]string{"url": "ftp://x"}); code != 400 {
			t.Fatalf("bad url: want 400, got %d", code)
		}
		code, body := call("POST", "/onboarding/analyze", map[string]string{"url": site.URL})
		if code != 200 || !strings.Contains(body, "event: step") || !strings.Contains(body, "event: result") || !strings.Contains(body, `"fallback":true`) {
			t.Fatalf("analyze stream: %d %s", code, body)
		}
		code, body = call("GET", "/brand-profile", nil)
		if code != 200 || !strings.Contains(body, `"product":"Acme"`) {
			t.Fatalf("brand profile: %d %s", code, body)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			docs, _ := q.ListDocuments(ctx, ws.ID)
			if len(docs) >= 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("KB not seeded: %d docs", len(docs))
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("put brand profile validates", func(t *testing.T) {
		if code, _ := call("PUT", "/brand-profile", map[string]any{"product": ""}); code != 400 {
			t.Fatalf("want 400, got %d", code)
		}
		code, body := call("PUT", "/brand-profile", map[string]any{
			"product": "Acme", "competitors": []string{"HubSpot"},
			"keywords":    map[string][]string{"category": {"crm for agencies"}, "problem": {"lost deals"}},
			"pain_points": []map[string]string{{"problem": "lost deals", "explanation": "x"}},
		})
		if code != 200 || !strings.Contains(body, "HubSpot") {
			t.Fatalf("put: %d %s", code, body)
		}
	})

	t.Run("build listeners streams proposals", func(t *testing.T) {
		if code, _ := call("POST", "/onboarding/listeners/build", map[string]any{"goals": []string{"nope"}}); code != 400 {
			t.Fatalf("bad goal: %d", code)
		}
		if code, _ := call("POST", "/onboarding/listeners/build", map[string]any{"goals": []string{"find_buyers"}, "platforms": []string{"myspace"}}); code != 400 {
			t.Fatalf("bad platform: %d", code)
		}
		code, body := call("POST", "/onboarding/listeners/build", map[string]any{"goals": []string{"find_buyers", "competitor_switchers"}, "platforms": []string{"hackernews"}})
		if code != 200 || strings.Count(body, "event: listener_proposal") != 2 || !strings.Contains(body, "event: query_tested") ||
			!strings.Contains(body, "event: samples") || !strings.Contains(body, "event: done") || !strings.Contains(body, "HubSpot alternative") {
			t.Fatalf("build stream: %d %s", code, body)
		}
	})

	t.Run("feedback, complete, calibration, first value", func(t *testing.T) {
		fb := map[string]string{"goal": "find_buyers", "url": "https://news.ycombinator.com/item?id=1", "text": "need a crm", "platform": "hackernews", "label": "up"}
		if code, body := call("POST", "/onboarding/listeners/feedback", fb); code != 200 {
			t.Fatalf("feedback: %d %s", code, body)
		}
		fb["label"] = "meh"
		if code, _ := call("POST", "/onboarding/listeners/feedback", fb); code != 400 {
			t.Fatalf("bad label: %d", code)
		}

		bad := map[string]any{"listeners": []map[string]any{{"goal": "find_buyers", "name": "x", "keywords": []string{}}}}
		if code, _ := call("POST", "/onboarding/complete", bad); code != 400 {
			t.Fatalf("listener without keywords must 400, got %d", code)
		}

		// A previous attempt failed after creating the first listener: the retry must reuse it.
		if _, err := q.CreateMonitoringProfile(ctx, database.CreateMonitoringProfileParams{WorkspaceID: ws.ID, Name: "Buyers", IsActive: true}); err != nil {
			t.Fatal(err)
		}
		code, body := call("POST", "/onboarding/complete", map[string]any{
			"brand_profile": map[string]any{"product": "Acme", "competitors": []string{"HubSpot", "Pipedrive"}},
			"listeners": []map[string]any{
				{"goal": "find_buyers", "name": "Buyers", "keywords": []string{"crm for agencies"}, "looks_for": "shoppers", "skips": "vendors", "platforms": []string{"hackernews"}},
				{"goal": "brand_mentions", "name": "Mentions", "keywords": []string{"acme crm"}, "looks_for": "mentions", "skips": "", "platforms": []string{"reddit"}},
			},
		})
		if code != 200 {
			t.Fatalf("complete: %d %s", code, body)
		}
		var res struct {
			ProfileIDs []string `json:"profile_ids"`
		}
		_ = json.Unmarshal([]byte(body), &res)
		if len(res.ProfileIDs) != 2 {
			t.Fatalf("profiles: %s", body)
		}
		p, err := q.GetMonitoringProfile(ctx, database.GetMonitoringProfileParams{ID: res.ProfileIDs[0], WorkspaceID: ws.ID})
		if err != nil || p.LooksFor != "shoppers" || p.Skips != "vendors" || p.Goal != "find_buyers" {
			t.Fatalf("profile intent not stored: %+v %v", p, err)
		}
		var buyers int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_profiles WHERE workspace_id = $1 AND name = 'Buyers'`, ws.ID).Scan(&buyers)
		if buyers != 1 {
			t.Fatalf("retry duplicated the listener: %d 'Buyers' profiles", buyers)
		}
		bp, _ := q.GetBrandProfile(ctx, ws.ID)
		if len(bp.Competitors) != 2 {
			t.Fatalf("competitors not persisted: %v", bp.Competitors)
		}
		scanner.mu.Lock()
		lb := scanner.lookback
		scanner.mu.Unlock()
		if lb != 7*24*time.Hour {
			t.Fatalf("backfill not triggered with 7 days: %v", lb)
		}
		ex, err := onboarding.ListCalibrationExamples(ctx, q, ws.ID, res.ProfileIDs[0])
		if err != nil || len(ex) != 1 || !ex[0].Relevant {
			t.Fatalf("calibration examples: %+v %v", ex, err)
		}
		// voice: completing onboarding seeds a default reply voice.
		if rp, err := q.GetPreferredResponseProfile(ctx, ws.ID); err != nil || rp.Name != "Default" {
			t.Fatalf("default response profile: %+v %v", rp, err)
		}
		if code, body := call("POST", "/onboarding/complete", map[string]any{"listeners": []map[string]any{{"goal": "find_buyers", "name": "B", "keywords": []string{"z"}}}}); code != 200 || !strings.Contains(body, "already_completed") {
			t.Fatalf("idempotency: %d %s", code, body)
		}

		code, body = call("GET", "/onboarding/first-value", nil)
		if code != 200 || !strings.Contains(body, `"tier":"none"`) || !strings.Contains(body, `"scanning":true`) {
			t.Fatalf("empty first value: %d %s", code, body)
		}
		if _, err := q.CreateMention(ctx, database.CreateMentionParams{
			WorkspaceID: ws.ID, Platform: "hackernews", PlatformID: slug, Url: "https://news.ycombinator.com/item?id=9",
			Content: "Anyone recommend a CRM for a small agency?", Status: database.MentionStatusNew,
			PlatformMetadata: []byte("{}"), EngagementMetrics: []byte("{}"), KeywordMatches: []string{"crm"},
		}); err != nil {
			t.Fatal(err)
		}
		code, body = call("GET", "/onboarding/first-value", nil)
		if code != 200 || !strings.Contains(body, `"tier":"recent"`) || !strings.Contains(body, `"loosened":true`) || !strings.Contains(body, "small agency") {
			t.Fatalf("loosened first value: %d %s", code, body)
		}
	})
}
