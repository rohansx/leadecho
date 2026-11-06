package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestEngineRunDB(t *testing.T) {
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

	n := time.Now().UnixNano()
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprint("org_wf_", n), Name: "wf test", Slug: fmt.Sprint("wf-", n)})
	if err != nil {
		t.Fatal(err)
	}
	newMention := func(i int, content string) Lead {
		m, err := q.CreateMention(ctx, database.CreateMentionParams{
			WorkspaceID: ws.ID, Platform: "reddit", PlatformID: fmt.Sprint("wf", n, i), Url: fmt.Sprintf("https://reddit.com/r/x/%d", i),
			Title: pgtype.Text{String: "Need a CRM", Valid: true}, Content: content, AuthorUsername: pgtype.Text{String: "alice", Valid: true},
			Status: database.MentionStatusNew, PlatformMetadata: []byte("{}"), EngagementMetrics: []byte("{}"),
			RelevanceScore: pgtype.Float4{Float32: 8, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return Lead{MentionID: m.ID, WorkspaceID: ws.ID, Platform: "reddit", Title: "Need a CRM", Content: content,
			URL: m.Url, Author: "alice", IntentTags: []string{"recommendation_request"}, Score: 8}
	}
	mkWorkflow := func(name string, active bool, trig Trigger, acts []ActionConfig, perHour int32) database.Workflow {
		tj, _ := json.Marshal(trig)
		aj, _ := json.Marshal(acts)
		status := database.WorkflowStatusPaused
		if active {
			status = database.WorkflowStatusActive
		}
		wf, err := q.CreateWorkflow(ctx, database.CreateWorkflowParams{WorkspaceID: ws.ID, Name: name, Status: status, TriggerConfig: tj, ActionChain: aj, MaxTriggersPerHour: perHour})
		if err != nil {
			t.Fatal(err)
		}
		return wf
	}

	drafts := 0
	e := NewEngine(q, zerolog.Nop())
	e.SetDrafter(func(ctx context.Context, wsID, mentionID string) error {
		drafts++
		if wsID != ws.ID {
			return errors.New("wrong workspace")
		}
		return nil
	})

	hot := mkWorkflow("hot", true, Trigger{MinScore: 7, IntentTags: []string{"recommendation_request"}},
		[]ActionConfig{{Type: ActionNotifySlack}, {Type: ActionAIDraft}, {Type: ActionCreateLead, Config: map[string]any{"tag": "hot"}}}, 2)
	paused := mkWorkflow("paused", false, Trigger{}, []ActionConfig{{Type: ActionAIDraft}}, 20)
	miss := mkWorkflow("miss", true, Trigger{Platforms: []string{"hackernews"}}, []ActionConfig{{Type: ActionAIDraft}}, 20)

	l1 := newMention(1, "What CRM do you all use for a 3 person team?")
	e.Run(ctx, l1)
	e.Run(ctx, l1) // same mention again: deduped

	runs, err := q.ListWorkflowExecutions(ctx, database.ListWorkflowExecutionsParams{WorkflowID: hot.ID, WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || drafts != 1 {
		t.Fatalf("runs = %d drafts = %d, want 1/1", len(runs), drafts)
	}
	var steps []StepResult
	_ = json.Unmarshal(runs[0].Steps, &steps)
	if runs[0].Status != database.ExecutionStatusCompleted || len(steps) != 3 ||
		steps[0].Status != "skipped" || steps[1].Status != "completed" || steps[2].Status != "completed" || steps[2].LeadID == "" {
		t.Fatalf("run = %s steps = %+v", runs[0].Status, steps)
	}
	lead, err := q.GetLead(ctx, database.GetLeadParams{ID: steps[2].LeadID, WorkspaceID: ws.ID})
	if err != nil || len(lead.Tags) != 2 || lead.Tags[1] != "hot" {
		t.Fatalf("lead = %+v err = %v", lead.Tags, err)
	}
	for _, wf := range []database.Workflow{paused, miss} {
		if r, _ := q.ListWorkflowExecutions(ctx, database.ListWorkflowExecutionsParams{WorkflowID: wf.ID, WorkspaceID: ws.ID}); len(r) != 0 {
			t.Fatalf("workflow %s should not have run", wf.Name)
		}
	}

	// Hourly cap is 2: the third new lead in the hour is skipped.
	e.Run(ctx, newMention(2, "Any CRM recommendations?"))
	e.Run(ctx, newMention(3, "Which CRM should I pick?"))
	if r, _ := q.ListWorkflowExecutions(ctx, database.ListWorkflowExecutionsParams{WorkflowID: hot.ID, WorkspaceID: ws.ID}); len(r) != 2 {
		t.Fatalf("runs after cap = %d, want 2", len(r))
	}

	// Another workspace can't read these runs.
	if r, _ := q.ListWorkflowExecutions(ctx, database.ListWorkflowExecutionsParams{WorkflowID: hot.ID, WorkspaceID: "00000000-0000-0000-0000-000000000000"}); len(r) != 0 {
		t.Fatal("runs leaked across workspaces")
	}

	// Preview sees the three scored posts and matches them all.
	res, err := Preview(ctx, q, ws.ID, Trigger{MinScore: 7})
	if err != nil || res.Checked != 3 || res.Matched != 3 || len(res.Samples) != 3 {
		t.Fatalf("preview = %+v err = %v", res, err)
	}
	if res, _ := Preview(ctx, q, ws.ID, Trigger{Keywords: []string{"recommendations"}}); res.Matched != 1 {
		t.Fatalf("keyword preview matched %d, want 1", res.Matched)
	}
}
