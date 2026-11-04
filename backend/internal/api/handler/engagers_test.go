package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/engagers"
)

type fakePuller struct {
	enabled bool
	got     engagers.Request
	rows    []engagers.Row
	err     error
}

func (f *fakePuller) Enabled() bool { return f.enabled }
func (f *fakePuller) Pull(_ context.Context, _ string, r engagers.Request) ([]engagers.Row, error) {
	f.got = r
	return f.rows, f.err
}

type fakeLeads struct{ known map[string]bool }

func (f fakeLeads) GetLead(_ context.Context, p database.GetLeadParams) (database.Lead, error) {
	if f.known[p.ID] {
		return database.Lead{ID: p.ID}, nil
	}
	return database.Lead{}, errors.New("no rows")
}

type fakeCRM struct {
	mu  sync.Mutex
	evs []crmsync.Event
}

func (f *fakeCRM) Emit(_ string, ev crmsync.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evs = append(f.evs, ev)
}

func post(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	return rec
}

func TestEngagersPull(t *testing.T) {
	const okURL = `"https://www.linkedin.com/feed/update/urn:li:activity:7508596155145043968/"`
	cases := []struct {
		name     string
		puller   *fakePuller
		body     string
		wantCode int
		wantErr  string
	}{
		{"treg off", &fakePuller{}, `{"post_url":` + okURL + `}`, http.StatusServiceUnavailable, "treg"},
		{"bad json", &fakePuller{enabled: true}, `{`, http.StatusBadRequest, "invalid JSON"},
		{"not linkedin", &fakePuller{enabled: true}, `{"post_url":"https://example.com/x"}`, http.StatusBadRequest, "post_url"},
		{"bad kind", &fakePuller{enabled: true}, `{"post_url":` + okURL + `,"kinds":["likes"]}`, http.StatusBadRequest, "kind"},
		{"treg miss", &fakePuller{enabled: true, err: engagers.ErrNoData}, `{"post_url":` + okURL + `}`, http.StatusBadGateway, "could not load"},
		{"timeout returns partial rows flagged truncated", &fakePuller{enabled: true, rows: []engagers.Row{{LeadID: "l1"}}, err: fmt.Errorf("%w: imported 1 of 5", engagers.ErrTruncated)}, `{"post_url":` + okURL + `}`, http.StatusOK, `"truncated":true`},
		{"ok", &fakePuller{enabled: true, rows: []engagers.Row{{Engager: engagers.Engager{Name: "Ann"}, LeadID: "l1"}}}, `{"post_url":` + okURL + `,"kinds":["comments"],"enrich":true}`, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &EngagersHandler{puller: tc.puller}
			rec := post(h.Pull, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("code %d: %s", rec.Code, rec.Body)
			}
			if tc.wantErr != "" && !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Fatalf("body %s missing %q", rec.Body, tc.wantErr)
			}
			if tc.wantCode != http.StatusOK {
				return
			}
			var resp struct {
				Data  []engagers.Row `json:"data"`
				Total int            `json:"total"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if resp.Total != 1 || resp.Data[0].LeadID != "l1" {
				t.Fatalf("resp %s", rec.Body)
			}
			g := tc.puller.got
			if tc.name != "ok" {
				return
			}
			if g.Ref.ID != "7508596155145043968" || len(g.Kinds) != 1 || g.Kinds[0] != "comments" || !g.Enrich {
				t.Fatalf("request = %+v", g)
			}
		})
	}
}

func TestEngagersPush(t *testing.T) {
	const a, b = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	cases := []struct {
		name      string
		body      string
		wantCode  int
		wantQueue int
	}{
		{"empty", `{"lead_ids":[]}`, http.StatusBadRequest, 0},
		{"bad uuid", `{"lead_ids":["x"]}`, http.StatusBadRequest, 0},
		{"other workspace's lead is skipped", `{"lead_ids":["` + a + `","` + b + `"]}`, http.StatusOK, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			crm := &fakeCRM{}
			h := &EngagersHandler{leads: fakeLeads{known: map[string]bool{a: true}}, crm: crm}
			rec := post(h.Push, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("code %d: %s", rec.Code, rec.Body)
			}
			if len(crm.evs) != tc.wantQueue {
				t.Fatalf("queued %d events", len(crm.evs))
			}
			if tc.wantQueue > 0 && (crm.evs[0].LeadID != a || crm.evs[0].Type != crmsync.EventLeadCreated) {
				t.Fatalf("event %+v", crm.evs[0])
			}
		})
	}
}

func TestNormalizeTrackedProfile(t *testing.T) {
	cases := []struct {
		platform, in, handle, url string
		ok                        bool
	}{
		{"linkedin", "https://www.linkedin.com/in/jane-doe-123/", "jane-doe-123", "https://www.linkedin.com/in/jane-doe-123", true},
		{"linkedin", "jane-doe", "jane-doe", "https://www.linkedin.com/in/jane-doe", true},
		{"linkedin", "https://evil.com/in/jane", "", "", false},
		{"x", "@levelsio", "levelsio", "https://x.com/levelsio", true},
		{"x", "https://twitter.com/levelsio?s=20", "levelsio", "https://x.com/levelsio", true},
		{"x", "not a handle!", "", "", false},
		{"reddit", "spez", "", "", false},
	}
	for _, c := range cases {
		h, u, err := NormalizeTrackedProfile(c.platform, c.in)
		if (err == nil) != c.ok || h != c.handle || u != c.url {
			t.Errorf("%s %q -> %q %q %v", c.platform, c.in, h, u, err)
		}
	}
}

type fakeTracked struct {
	rows    []database.TrackedProfile
	created database.CreateTrackedProfileParams
	deleted int64
}

func (f *fakeTracked) ListTrackedProfiles(context.Context, string) ([]database.TrackedProfile, error) {
	return f.rows, nil
}
func (f *fakeTracked) CreateTrackedProfile(_ context.Context, p database.CreateTrackedProfileParams) (database.TrackedProfile, error) {
	f.created = p
	return database.TrackedProfile{ID: "tp1", WorkspaceID: p.WorkspaceID, Platform: p.Platform, Handle: p.Handle, Url: p.Url}, nil
}
func (f *fakeTracked) DeleteTrackedProfile(context.Context, database.DeleteTrackedProfileParams) (int64, error) {
	return f.deleted, nil
}

func TestTrackedProfilesHandler(t *testing.T) {
	f := &fakeTracked{}
	h := &TrackedProfilesHandler{q: f}

	rec := post(h.Create, `{"platform":"x","handle":"@levelsio"}`)
	if rec.Code != http.StatusCreated || f.created.Handle != "levelsio" || f.created.Platform != "x" {
		t.Fatalf("create %d %s %+v", rec.Code, rec.Body, f.created)
	}
	if rec := post(h.Create, `{"platform":"x","handle":"!!"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid handle accepted: %d", rec.Code)
	}
	f.rows = make([]database.TrackedProfile, maxTrackedProfiles)
	if rec := post(h.Create, `{"platform":"x","handle":"other"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit not enforced: %d", rec.Code)
	}

	del := func(id string) int {
		r := httptest.NewRequest(http.MethodDelete, "/", nil)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", id)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
		rec := httptest.NewRecorder()
		h.Delete(rec, r)
		return rec.Code
	}
	if code := del("nope"); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
	if code := del("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}
	f.deleted = 1
	if code := del("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
}
