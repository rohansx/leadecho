package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"leadecho/internal/database"
	"leadecho/internal/tools"
)

type publicFakeQ struct {
	database.Querier
	query string
	lim   int32
}

func (f *publicFakeQ) SearchMentions(_ context.Context, a database.SearchMentionsParams) ([]database.Mention, error) {
	f.query, f.lim = a.Query, a.Lim
	return []database.Mention{{ID: "m1", Platform: "reddit"}}, nil
}
func (f *publicFakeQ) ListMentions(_ context.Context, a database.ListMentionsParams) ([]database.Mention, error) {
	f.lim = a.Lim
	return []database.Mention{}, nil
}
func (f *publicFakeQ) ListLeadsByStage(_ context.Context, a database.ListLeadsByStageParams) ([]database.Lead, error) {
	f.query = string(a.Stage)
	return []database.Lead{{ID: "l1", Stage: a.Stage}}, nil
}

func TestPublicAPI(t *testing.T) {
	q := &publicFakeQ{}
	h := NewPublicAPIHandler(tools.New(tools.Deps{Q: q}), "9.9.9")
	h.auth = func(context.Context) (string, []string) { return "ws-1", []string{"read"} }

	tests := []struct {
		name   string
		fn     http.HandlerFunc
		url    string
		want   int
		listed string
	}{
		{"mentions search", h.Mentions, "/?q=crm&limit=5", 200, "mentions"},
		{"mentions default", h.Mentions, "/", 200, "mentions"},
		{"mentions bad limit", h.Mentions, "/?limit=abc", 400, ""},
		{"mentions bad status", h.Mentions, "/?status=gone", 400, ""},
		{"leads by stage", h.Leads, "/?stage=qualified", 200, "leads"},
		{"leads bad stage", h.Leads, "/?stage=won", 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.fn(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if rec.Code != tt.want {
				t.Fatalf("status %d want %d: %s", rec.Code, tt.want, rec.Body)
			}
			if tt.listed != "" {
				var out map[string]any
				json.Unmarshal(rec.Body.Bytes(), &out)
				if _, ok := out[tt.listed].([]any); !ok {
					t.Fatalf("missing %s list: %s", tt.listed, rec.Body)
				}
			}
		})
	}
	rec := httptest.NewRecorder()
	h.Mentions(rec, httptest.NewRequest(http.MethodGet, "/?q=crm&limit=5", nil))
	if q.query != "crm" || q.lim != 5 {
		t.Fatalf("args not forwarded: %q %d", q.query, q.lim)
	}

	rec = httptest.NewRecorder()
	h.Probe(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var probe map[string]any
	json.Unmarshal(rec.Body.Bytes(), &probe)
	if rec.Code != 200 || probe["service"] != "leadecho" || probe["version"] != "9.9.9" || probe["status"] != "ok" {
		t.Fatalf("probe: %d %s", rec.Code, rec.Body)
	}
}
