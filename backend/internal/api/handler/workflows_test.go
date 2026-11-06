package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/database"
)

type fakeWorkflowStore struct {
	WorkflowStore // unimplemented methods panic if a test reaches them
	created       *database.CreateWorkflowParams
	count         int64
}

func (f *fakeWorkflowStore) CountWorkflows(context.Context, string) (int64, error) { return f.count, nil }
func (f *fakeWorkflowStore) CreateWorkflow(_ context.Context, p database.CreateWorkflowParams) (database.Workflow, error) {
	f.created = &p
	return database.Workflow{ID: "wf1", WorkspaceID: p.WorkspaceID, Name: p.Name, Status: p.Status,
		TriggerConfig: p.TriggerConfig, ActionChain: p.ActionChain, MaxTriggersPerHour: p.MaxTriggersPerHour, CreatedAt: time.Now()}, nil
}

func postWorkflow(h *WorkflowHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/workflows", strings.NewReader(body))
	req = req.WithContext(middleware.WithClaims(req.Context(), &auth.Claims{WorkspaceID: "ws1", UserID: "11111111-1111-1111-1111-111111111111"}))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

func TestWorkflowCreate(t *testing.T) {
	publicOnly := func(_ context.Context, raw string) error {
		if strings.Contains(raw, "169.254") {
			return errors.New("private")
		}
		return nil
	}
	tests := []struct {
		name     string
		body     string
		count    int64
		wantCode int
		wantErr  string
	}{
		{"valid", `{"name":"Hot buyers","enabled":true,"trigger":{"min_score":7,"intent_tags":["buying_intent"]},"actions":[{"type":"notify_slack"},{"type":"ai_draft"}]}`, 0, 201, ""},
		{"bad json", `{`, 0, 400, "invalid JSON"},
		{"no actions", `{"name":"x","actions":[]}`, 0, 400, "actions"},
		{"private webhook", `{"name":"x","actions":[{"type":"webhook","config":{"url":"https://169.254.169.254/x"}}]}`, 0, 400, "public https"},
		{"limit", `{"name":"x","actions":[{"type":"ai_draft"}]}`, maxWorkflowsPerWorkspace, 400, "limit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeWorkflowStore{count: tc.count}
			h := &WorkflowHandler{q: store, validateURL: publicOnly}
			rec := postWorkflow(h, tc.body)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
			}
			if tc.wantCode != 201 {
				return
			}
			if store.created.WorkspaceID != "ws1" || store.created.Status != database.WorkflowStatusActive || store.created.MaxTriggersPerHour != 20 {
				t.Fatalf("created = %+v", store.created)
			}
			var v WorkflowView
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || !v.Enabled || len(v.Actions) != 2 || v.Trigger.MinScore != 7 {
				t.Fatalf("view = %+v err = %v", v, err)
			}
		})
	}
}
