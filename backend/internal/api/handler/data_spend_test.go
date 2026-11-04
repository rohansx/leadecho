package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type fakeSpend struct {
	got  database.TregSpendByToolParams
	rows []database.TregSpendByToolRow
	err  error
}

func (f *fakeSpend) TregSpendByTool(_ context.Context, p database.TregSpendByToolParams) ([]database.TregSpendByToolRow, error) {
	f.got = p
	return f.rows, f.err
}

func TestMonthStartUTC(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800)
	cases := map[time.Time]time.Time{
		time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC): time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 2, 0, 0, 0, ist):       time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), // still Nov 30 in UTC
	}
	for in, want := range cases {
		if got := monthStartUTC(in); !got.Equal(want) {
			t.Errorf("monthStartUTC(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestDataSpend(t *testing.T) {
	f := &fakeSpend{rows: []database.TregSpendByToolRow{
		{Tool: "anyapi.linkedin.search.posts", CostMicro: 4000, Calls: 2},
		{Tool: "treg.x.search.posts", CostMicro: 1500, Calls: 3},
	}}
	h := &DataSpendHandler{q: f, now: func() time.Time { return time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC) }}
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/api/v1/usage/data-spend", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if f.got.WorkspaceID != middleware.WorkspaceID(context.Background()) || !f.got.Since.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("query params = %+v", f.got)
	}
	var resp DataSpendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.TotalCostMicro != 5500 || resp.TotalCostUSD != 0.0055 || resp.Calls != 5 || len(resp.ByTool) != 2 || resp.PeriodStart != "2026-11-01T00:00:00Z" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDataSpendEmptyAndError(t *testing.T) {
	h := &DataSpendHandler{q: &fakeSpend{}, now: time.Now}
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var resp DataSpendResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != 200 || resp.ByTool == nil || resp.TotalCostMicro != 0 {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body)
	}

	h = &DataSpendHandler{q: &fakeSpend{err: errors.New("db down")}, now: time.Now}
	rec = httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("error: %d", rec.Code)
	}
}
