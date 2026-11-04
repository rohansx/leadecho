package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"leadecho/internal/database"
	"leadecho/internal/tools"
)

const (
	ws  = "11111111-1111-1111-1111-111111111111"
	aID = "22222222-2222-2222-2222-222222222222"
)

type toolCall struct {
	Name   string
	Args   map[string]any
	Scopes []string
}

// fakeTools answers tool calls from per-tool handlers and records them.
type fakeTools struct {
	mu       sync.Mutex
	calls    []toolCall
	handlers map[string]func(args map[string]any) (any, error)
}

func (f *fakeTools) Call(_ context.Context, wsID string, scopes []string, name string, raw json.RawMessage) (any, error) {
	if wsID != ws {
		panic("wrong workspace " + wsID)
	}
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	f.mu.Lock()
	f.calls = append(f.calls, toolCall{Name: name, Args: args, Scopes: scopes})
	h := f.handlers[name]
	f.mu.Unlock()
	if h == nil {
		return nil, tools.ErrUnknownTool
	}
	return h(args)
}

func (f *fakeTools) called(name string) []toolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []toolCall
	for _, c := range f.calls {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func mentionsResult(ms ...tools.MentionView) (any, error) {
	return map[string]any{"mentions": ms, "count": len(ms)}, nil
}

var idNames = map[string]string{}

// uuidFor gives each test name a stable UUID (mention ids must parse).
func uuidFor(name string) string {
	var n uint32
	for _, c := range name {
		n = n*31 + uint32(c)
	}
	id := fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
	idNames[id] = name
	return id
}

func mv(name string, score float32, intent, status string, created time.Time) tools.MentionView {
	id := uuidFor(name)
	title := "post " + name
	author := "author-" + name
	v := tools.MentionView{ID: id, Platform: "reddit", URL: "https://x.test/" + id, Title: &title, Author: &author,
		Status: status, CreatedAt: created}
	if score >= 0 {
		v.Score = &score
	}
	if intent != "" {
		v.Intent = &intent
	}
	return v
}

// fakeQ implements the database.Querier subset automations use; any other
// method panics via the nil embedded interface.
type fakeQ struct {
	database.Querier
	mu sync.Mutex

	archived    []string
	leadsFor    map[string]bool // mention id -> already a lead
	created     []database.CreateLeadParams
	kwRows      []database.AutomationKeywordValueRow
	automations []database.Automation
	claimed     map[string]bool // automation|period
	runs        []database.AutomationRun
	finished    []database.FinishAutomationRunParams
	lastRun     map[string]time.Time
	notes       []database.CreateNotificationParams
}

func (f *fakeQ) UpdateMentionStatus(_ context.Context, a database.UpdateMentionStatusParams) (database.Mention, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.WorkspaceID != ws {
		return database.Mention{}, pgx.ErrNoRows
	}
	f.archived = append(f.archived, a.ID)
	return database.Mention{ID: a.ID, Status: a.Status}, nil
}

func (f *fakeQ) GetLeadByMention(_ context.Context, a database.GetLeadByMentionParams) (database.Lead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, _ := a.MentionID.Value()
	if s, _ := id.(string); f.leadsFor[idNames[s]] && a.WorkspaceID == ws {
		return database.Lead{ID: "existing"}, nil
	}
	return database.Lead{}, pgx.ErrNoRows
}

func (f *fakeQ) CreateLead(_ context.Context, a database.CreateLeadParams) (database.Lead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, a)
	return database.Lead{ID: "lead-" + string(rune('a'+len(f.created)-1)), WorkspaceID: a.WorkspaceID}, nil
}

func (f *fakeQ) AutomationKeywordValue(_ context.Context, a database.AutomationKeywordValueParams) ([]database.AutomationKeywordValueRow, error) {
	if a.WorkspaceID != ws {
		return nil, nil
	}
	return f.kwRows, nil
}

func (f *fakeQ) ListEnabledAutomations(context.Context) ([]database.Automation, error) {
	return f.automations, nil
}

func (f *fakeQ) ClaimScheduledAutomationRun(_ context.Context, a database.ClaimScheduledAutomationRunParams) (database.AutomationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimed == nil {
		f.claimed = map[string]bool{}
	}
	key := a.AutomationID + "|" + a.PeriodStart.UTC().String()
	if f.claimed[key] {
		return database.AutomationRun{}, pgx.ErrNoRows
	}
	f.claimed[key] = true
	r := database.AutomationRun{ID: "run-" + key, WorkspaceID: a.WorkspaceID, AutomationID: a.AutomationID, Trigger: "schedule", Status: "running"}
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeQ) CreateManualAutomationRun(_ context.Context, a database.CreateManualAutomationRunParams) (database.AutomationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := database.AutomationRun{ID: "manual-run", WorkspaceID: a.WorkspaceID, AutomationID: a.AutomationID, Trigger: "manual", Status: "running"}
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeQ) FinishAutomationRun(_ context.Context, a database.FinishAutomationRunParams) (database.AutomationRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, a)
	return database.AutomationRun{ID: a.ID, WorkspaceID: a.WorkspaceID, Status: a.Status, Output: a.Output, Error: a.Error}, nil
}

func (f *fakeQ) SetAutomationLastRun(_ context.Context, a database.SetAutomationLastRunParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastRun == nil {
		f.lastRun = map[string]time.Time{}
	}
	f.lastRun[a.ID] = a.LastRunAt.Time
	return nil
}

func (f *fakeQ) CreateNotification(_ context.Context, a database.CreateNotificationParams) (database.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, a)
	return database.Notification{}, nil
}

func (f *fakeQ) GetWorkspaceOwnerEmail(context.Context, string) (string, error) {
	return "owner@x.test", nil
}
