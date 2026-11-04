package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/database"
	"leadecho/internal/mutes"
)

type sourcesFakeQ struct {
	database.Querier
	mutes     []database.Mute
	deleted   database.DeleteMuteParams
	cadences  []database.ListKeywordCadencesRow
	updated   database.UpdateKeywordCadenceParams
	updateErr error
}

func (f *sourcesFakeQ) ListMutes(_ context.Context, ws string) ([]database.Mute, error) {
	return f.mutes, nil
}
func (f *sourcesFakeQ) CreateMute(_ context.Context, p database.CreateMuteParams) (database.Mute, error) {
	m := database.Mute{ID: "m1", WorkspaceID: p.WorkspaceID, Kind: p.Kind, Value: p.Value, CreatedBy: p.CreatedBy}
	f.mutes = append(f.mutes, m)
	return m, nil
}
func (f *sourcesFakeQ) DeleteMute(_ context.Context, p database.DeleteMuteParams) (int64, error) {
	f.deleted = p
	if p.ID == "22222222-2222-2222-2222-222222222222" {
		return 1, nil
	}
	return 0, nil
}
func (f *sourcesFakeQ) ListKeywordCadences(_ context.Context, ws string) ([]database.ListKeywordCadencesRow, error) {
	return f.cadences, nil
}
func (f *sourcesFakeQ) UpdateKeywordCadence(_ context.Context, p database.UpdateKeywordCadenceParams) (database.UpdateKeywordCadenceRow, error) {
	f.updated = p
	if f.updateErr != nil {
		return database.UpdateKeywordCadenceRow{}, f.updateErr
	}
	return database.UpdateKeywordCadenceRow{ID: p.ID, Cadence: p.Cadence}, nil
}

func routed(method, pattern, target, body string, h http.HandlerFunc) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Method(method, pattern, h)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withWorkspace(httptest.NewRequest(method, target, strings.NewReader(body)), "ws-1"))
	return rec
}

func TestMuteHandler(t *testing.T) {
	f := &sourcesFakeQ{}
	h := NewMuteHandler(mutes.NewStore(f))

	tests := []struct {
		name, body string
		want       int
	}{
		{"valid", `{"kind":"author","value":"@Spammer"}`, http.StatusCreated},
		{"bad kind", `{"kind":"email","value":"x"}`, http.StatusBadRequest},
		{"blank value", `{"kind":"domain","value":"  "}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := routed(http.MethodPost, "/mutes", "/mutes", tt.body, h.Create)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
	if len(f.mutes) != 1 || f.mutes[0].Value != "spammer" || f.mutes[0].WorkspaceID != "ws-1" || !f.mutes[0].CreatedBy.Valid {
		t.Fatalf("stored = %+v", f.mutes)
	}

	rec := routed(http.MethodGet, "/mutes", "/mutes", "", h.List)
	var list []MuteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Kind != "author" {
		t.Errorf("list = %s", rec.Body.String())
	}

	if rec := routed(http.MethodDelete, "/mutes/{id}", "/mutes/not-a-uuid", "", h.Delete); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d", rec.Code)
	}
	if rec := routed(http.MethodDelete, "/mutes/{id}", "/mutes/33333333-3333-3333-3333-333333333333", "", h.Delete); rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d", rec.Code)
	}
	if rec := routed(http.MethodDelete, "/mutes/{id}", "/mutes/22222222-2222-2222-2222-222222222222", "", h.Delete); rec.Code != http.StatusOK {
		t.Errorf("delete status = %d", rec.Code)
	}
	if f.deleted.WorkspaceID != "ws-1" {
		t.Errorf("delete not workspace scoped: %+v", f.deleted)
	}
}

func TestKeywordCadenceUpdate(t *testing.T) {
	const id = "44444444-4444-4444-4444-444444444444"
	tests := []struct {
		name, target, body string
		err                error
		want               int
	}{
		{"valid", "/keywords/" + id + "/cadence", `{"cadence":{"twitter":"6h","reddit":"30m"}}`, nil, http.StatusOK},
		{"bad value", "/keywords/" + id + "/cadence", `{"cadence":{"twitter":"2m"}}`, nil, http.StatusBadRequest},
		{"bad platform", "/keywords/" + id + "/cadence", `{"cadence":{"myspace":"1h"}}`, nil, http.StatusBadRequest},
		{"bad id", "/keywords/nope/cadence", `{"cadence":{}}`, nil, http.StatusBadRequest},
		{"not found", "/keywords/" + id + "/cadence", `{"cadence":{}}`, pgx.ErrNoRows, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &sourcesFakeQ{updateErr: tt.err}
			h := NewKeywordCadenceHandler(f)
			rec := routed(http.MethodPut, "/keywords/{id}/cadence", tt.target, tt.body, h.Update)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusOK && (f.updated.WorkspaceID != "ws-1" || !strings.Contains(string(f.updated.Cadence), `"twitter":"6h"`)) {
				t.Errorf("updated = %+v %s", f.updated, f.updated.Cadence)
			}
		})
	}
}

func TestKeywordCostPreview(t *testing.T) {
	f := &sourcesFakeQ{cadences: []database.ListKeywordCadencesRow{
		{ID: "k1", Term: "crm", Platforms: []string{"twitter", "reddit"}, IsActive: true, Cadence: []byte(`{"twitter":"1h"}`)},
		{ID: "k2", Term: "old", Platforms: []string{"linkedin"}, IsActive: false, Cadence: []byte(`{}`)},
	}}
	rec := routed(http.MethodGet, "/keywords/cost-preview", "/keywords/cost-preview", "", NewKeywordCadenceHandler(f).CostPreview)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp costPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Keywords) != 2 || resp.Keywords[0].Platforms[0].CallsPerMonth != 720 {
		t.Fatalf("resp = %+v", resp)
	}
	// Inactive keywords are listed but do not count toward the total.
	if resp.USDPerMonth != 0.54 {
		t.Errorf("total = %v, want 0.54", resp.USDPerMonth)
	}
	if resp.Rates["linkedin"] != 0.002 || len(resp.Options) == 0 {
		t.Errorf("rates/options missing: %+v", resp)
	}
}
