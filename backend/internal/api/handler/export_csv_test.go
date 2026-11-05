package handler

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
)

func TestCSVSafe(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"=HYPERLINK(\"x\")", "'=HYPERLINK(\"x\")"},
		{"+1", "'+1"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\t=x", "'\t=x"},
		{"\r=x", "'\r=x"},
		{"", ""},
		{"a=b", "a=b"},
		{"\n=HYPERLINK(\"x\")", "'\n=HYPERLINK(\"x\")"},
		{"  =1+1", "'  =1+1"},
		{" hello", " hello"},
	}
	for _, tt := range tests {
		if got := csvSafe(tt.in); got != tt.want {
			t.Errorf("csvSafe(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// exportFakeQ serves pages of mentions/leads and records the params it saw.
type exportFakeQ struct {
	database.Querier
	mentions     []database.Mention
	leads        []database.Lead
	gotMention   []database.ListMentionsComposedParams
	gotLeadStage string
	gotLeadWS    string
}

func (f *exportFakeQ) ListMentionsComposed(_ context.Context, p database.ListMentionsComposedParams) ([]database.Mention, error) {
	f.gotMention = append(f.gotMention, p)
	return page(f.mentions, p.Off, p.Lim), nil
}

func (f *exportFakeQ) ListLeads(_ context.Context, p database.ListLeadsParams) ([]database.Lead, error) {
	f.gotLeadWS = p.WorkspaceID
	return page(f.leads, p.Off, p.Lim), nil
}

func (f *exportFakeQ) ListLeadsByStage(_ context.Context, p database.ListLeadsByStageParams) ([]database.Lead, error) {
	f.gotLeadWS, f.gotLeadStage = p.WorkspaceID, string(p.Stage)
	return page(f.leads, p.Off, p.Lim), nil
}

func page[T any](all []T, off, lim int32) []T {
	if int(off) >= len(all) {
		return nil
	}
	end := int(off + lim)
	if end > len(all) {
		end = len(all)
	}
	return all[off:end]
}

func TestExportMentionsCSV(t *testing.T) {
	ms := make([]database.Mention, exportPageSize+3)
	for i := range ms {
		ms[i] = database.Mention{ID: "m", Platform: "reddit", Url: "https://x", Content: "plain", Status: "new", CreatedAt: time.Unix(0, 0).UTC()}
	}
	ms[0].Content = "=cmd|' /C calc'!A0"
	ms[0].Title = pgtype.Text{String: "@title", Valid: true}
	f := &exportFakeQ{mentions: ms}
	h := NewExportHandler(f)

	req := withWorkspace(httptest.NewRequest(http.MethodGet, "/mentions/export.csv?platform=reddit&status=new&search=crm", nil), "ws-1")
	rec := httptest.NewRecorder()
	h.Mentions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "mentions-") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	rows, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(ms)+1 {
		t.Fatalf("rows = %d, want %d (header + all pages)", len(rows), len(ms)+1)
	}
	if rows[0][0] != "id" {
		t.Errorf("header = %v", rows[0])
	}
	joined := strings.Join(rows[1], "|")
	if !strings.Contains(joined, "'=cmd") || !strings.Contains(joined, "'@title") {
		t.Errorf("formula cells not escaped: %v", rows[1])
	}
	p := f.gotMention[0]
	if p.WorkspaceID != "ws-1" || p.Platform != "reddit" || p.Status != "new" || p.Search != "crm" {
		t.Errorf("filters not passed: %+v", p)
	}
	if len(f.gotMention) != 2 {
		t.Errorf("pages fetched = %d, want 2", len(f.gotMention))
	}
}

func TestExportMentionsCSVRejectsBadFilter(t *testing.T) {
	h := NewExportHandler(&exportFakeQ{})
	rec := httptest.NewRecorder()
	h.Mentions(rec, withWorkspace(httptest.NewRequest(http.MethodGet, "/mentions/export.csv?platform=myspace", nil), "ws"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestExportLeadsCSV(t *testing.T) {
	f := &exportFakeQ{leads: []database.Lead{{
		ID: "l1", Stage: "qualified", ContactName: pgtype.Text{String: "+Bob", Valid: true},
		Tags: []string{"a", "b"}, CreatedAt: time.Unix(0, 0).UTC(),
	}}}
	h := NewExportHandler(f)

	rec := httptest.NewRecorder()
	h.Leads(rec, withWorkspace(httptest.NewRequest(http.MethodGet, "/leads/export.csv?stage=qualified", nil), "ws-2"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if f.gotLeadWS != "ws-2" || f.gotLeadStage != "qualified" {
		t.Errorf("ws=%q stage=%q", f.gotLeadWS, f.gotLeadStage)
	}
	rows, _ := csv.NewReader(rec.Body).ReadAll()
	if len(rows) != 2 || !strings.Contains(strings.Join(rows[1], "|"), "'+Bob") || !strings.Contains(strings.Join(rows[1], "|"), "a;b") {
		t.Errorf("rows = %v", rows)
	}

	rec = httptest.NewRecorder()
	h.Leads(rec, withWorkspace(httptest.NewRequest(http.MethodGet, "/leads/export.csv?stage=bogus", nil), "ws-2"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad stage status = %d, want 400", rec.Code)
	}
}
