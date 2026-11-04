package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"leadecho/internal/database"
)

const (
	autoWS    = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	autoOther = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	autoID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

type fakeAutoQ struct {
	database.Querier
	rows    map[string]database.Automation
	created *database.CreateAutomationParams
	running bool
}

func (f *fakeAutoQ) ListAutomations(_ context.Context, ws string) ([]database.Automation, error) {
	out := []database.Automation{}
	for _, a := range f.rows {
		if a.WorkspaceID == ws {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeAutoQ) GetAutomation(_ context.Context, a database.GetAutomationParams) (database.Automation, error) {
	row, ok := f.rows[a.ID]
	if !ok || row.WorkspaceID != a.WorkspaceID {
		return database.Automation{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeAutoQ) CreateAutomation(_ context.Context, a database.CreateAutomationParams) (database.Automation, error) {
	f.created = &a
	return database.Automation{ID: autoID, WorkspaceID: a.WorkspaceID, Name: a.Name, Kind: a.Kind, Frequency: a.Frequency,
		RunTime: a.RunTime, Weekday: a.Weekday, Timezone: a.Timezone, Enabled: a.Enabled, Config: a.Config}, nil
}

func (f *fakeAutoQ) ListAutomationRuns(_ context.Context, a database.ListAutomationRunsParams) ([]database.AutomationRun, error) {
	if a.WorkspaceID != autoWS || !f.running {
		return []database.AutomationRun{}, nil
	}
	return []database.AutomationRun{{ID: "r1", Status: "running", StartedAt: time.Now(), Output: []byte(`{}`)}}, nil
}

type fakeAutoRunner struct{ ran int }

func (f *fakeAutoRunner) RunNow(_ context.Context, a database.Automation) (database.AutomationRun, error) {
	f.ran++
	return database.AutomationRun{ID: "r2", AutomationID: a.ID, Trigger: "manual", Status: "succeeded", Output: []byte(`{"summary":"ok"}`)}, nil
}

func autoReq(method, body, id, ws string) *http.Request {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	if id != "" {
		r = withURLParam(r, "id", id)
	}
	return withWorkspace(r, ws)
}

func TestAutomationCreateValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `{`, http.StatusBadRequest},
		{"unknown field", `{"name":"x","kind":"lead_sweep","bogus":1}`, http.StatusBadRequest},
		{"bad kind", `{"name":"x","kind":"nope"}`, http.StatusBadRequest},
		{"bad schedule", `{"name":"x","kind":"lead_sweep","schedule":{"frequency":"hourly"}}`, http.StatusBadRequest},
		{"ok", `{"name":"Sweep","kind":"lead_sweep","enabled":true,"schedule":{"frequency":"weekly","time":"08:00","weekday":"friday","timezone":"UTC"},"config":{"push_to_crm":true}}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeAutoQ{}
			rec := httptest.NewRecorder()
			NewAutomationHandler(q, &fakeAutoRunner{}).Create(rec, autoReq(http.MethodPost, tt.body, "", autoWS))
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != http.StatusCreated {
				if q.created != nil {
					t.Fatal("invalid input must not be stored")
				}
				return
			}
			if q.created.WorkspaceID != autoWS || q.created.Frequency != "weekly" || !strings.Contains(string(q.created.Config), `"push_to_crm":true`) {
				t.Fatalf("created = %+v (%s)", q.created, q.created.Config)
			}
			var v AutomationView
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Schedule.Weekday != "friday" || v.Config.MinScore != 6 {
				t.Fatalf("view = %+v err=%v", v, err)
			}
		})
	}
}

func TestAutomationRunAndScoping(t *testing.T) {
	row := database.Automation{ID: autoID, WorkspaceID: autoWS, Name: "n", Kind: "lead_sweep", Frequency: "daily", RunTime: "09:30", Weekday: "monday", Timezone: "UTC", Config: []byte(`{}`)}
	tests := []struct {
		name    string
		id, ws  string
		running bool
		want    int
		wantRan int
	}{
		{"bad id", "nope", autoWS, false, http.StatusBadRequest, 0},
		{"other workspace", autoID, autoOther, false, http.StatusNotFound, 0},
		{"already running", autoID, autoWS, true, http.StatusConflict, 0},
		{"runs", autoID, autoWS, false, http.StatusOK, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeAutoQ{rows: map[string]database.Automation{autoID: row}, running: tt.running}
			run := &fakeAutoRunner{}
			rec := httptest.NewRecorder()
			NewAutomationHandler(q, run).Run(rec, autoReq(http.MethodPost, "", tt.id, tt.ws))
			if rec.Code != tt.want || run.ran != tt.wantRan {
				t.Fatalf("status %d ran %d, want %d/%d: %s", rec.Code, run.ran, tt.want, tt.wantRan, rec.Body)
			}
		})
	}
}

func TestSkillsEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	ListSkills(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var out []struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out) < 7 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}
