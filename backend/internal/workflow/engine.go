package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
)

// Drafter writes an AI reply draft for a mention (reply.Drafter, adapted).
type Drafter func(ctx context.Context, wsID, mentionID string) error

// Engine runs a workspace's real-time workflows against each newly scored lead.
type Engine struct {
	q      *database.Queries
	acts   *actions
	logger zerolog.Logger
}

func NewEngine(q *database.Queries, logger zerolog.Logger) *Engine {
	return &Engine{q: q, acts: newActions(q), logger: logger.With().Str("component", "workflow_engine").Logger()}
}

func (e *Engine) SetDrafter(d Drafter)        { e.acts.draft = d }
func (e *Engine) SetCRM(c CRMEmitter)         { e.acts.crm = c }
func (e *Engine) SetMail(apiKey, from string) { e.acts.mail.APIKey, e.acts.mail.From = apiKey, from }
func (e *Engine) SetAppURL(u string)          { e.acts.appURL = u }

// Run evaluates every active workflow of the lead's workspace. Failures are
// logged per workflow; one bad workflow never stops the others.
func (e *Engine) Run(ctx context.Context, l Lead) {
	wfs, err := e.q.ListActiveWorkflowsByWorkspace(ctx, l.WorkspaceID)
	if err != nil {
		e.logger.Error().Err(err).Str("workspace_id", l.WorkspaceID).Msg("list workflows")
		return
	}
	for _, wf := range wfs {
		if err := e.runOne(ctx, wf, l); err != nil {
			e.logger.Error().Err(err).Str("workflow_id", wf.ID).Str("mention_id", l.MentionID).Msg("workflow run failed")
		}
	}
}

// EvaluateAndTrigger is the streams-consumer entry point.
func (e *Engine) EvaluateAndTrigger(ctx context.Context, p events.WorkflowTriggerRequestedPayload) error {
	e.Run(ctx, Lead{
		MentionID: p.MentionID, WorkspaceID: p.WorkspaceID, Platform: p.Platform, Title: p.Title,
		Content: p.Content, URL: p.URL, Author: p.Author, Intent: p.Intent, Score: float64(p.RelevanceScore),
	})
	return nil
}

func (e *Engine) runOne(ctx context.Context, wf database.Workflow, l Lead) error {
	var t Trigger
	if err := json.Unmarshal(wf.TriggerConfig, &t); err != nil {
		return fmt.Errorf("parse trigger: %w", err)
	}
	if !t.Matches(l) {
		return nil
	}
	mention := pgUUID(l.MentionID)
	done, err := e.q.HasWorkflowExecutionForMention(ctx, database.HasWorkflowExecutionForMentionParams{WorkflowID: wf.ID, MentionID: mention})
	if err != nil || done {
		return err
	}
	n, err := e.q.CountWorkflowExecutionsSince(ctx, database.CountWorkflowExecutionsSinceParams{WorkflowID: wf.ID, Since: time.Now().Add(-time.Hour)})
	if err != nil {
		return fmt.Errorf("rate check: %w", err)
	}
	if n >= int64(wf.MaxTriggersPerHour) {
		e.logger.Warn().Str("workflow_id", wf.ID).Int64("runs_last_hour", n).Msg("workflow hourly cap reached; skipping")
		return nil
	}
	var chain []ActionConfig
	if err := json.Unmarshal(wf.ActionChain, &chain); err != nil {
		return fmt.Errorf("parse actions: %w", err)
	}
	exec, err := e.q.CreateWorkflowExecution(ctx, database.CreateWorkflowExecutionParams{
		WorkflowID: wf.ID, MentionID: mention, Status: database.ExecutionStatusRunning, Steps: []byte("[]"),
	})
	if err != nil {
		return fmt.Errorf("create execution: %w", err)
	}
	if _, err := e.q.IncrementWorkflowTriggerCount(ctx, wf.ID); err != nil {
		e.logger.Warn().Err(err).Str("workflow_id", wf.ID).Msg("increment trigger count")
	}

	steps, failed := e.acts.runChain(ctx, wf, l, chain)
	status, errMsg := database.ExecutionStatusCompleted, pgtype.Text{}
	if failed > 0 {
		status = database.ExecutionStatusFailed
		errMsg = pgtype.Text{String: fmt.Sprintf("%d of %d actions failed", failed, len(steps)), Valid: true}
	}
	stepsJSON, _ := json.Marshal(steps)
	_, err = e.q.UpdateWorkflowExecution(ctx, database.UpdateWorkflowExecutionParams{
		ID: exec.ID, Status: status, CurrentStep: int32(len(steps)), Steps: stepsJSON, ErrorMessage: errMsg,
		CompletedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	e.logger.Info().Str("workflow_id", wf.ID).Str("mention_id", l.MentionID).Str("status", string(status)).Msg("workflow ran")
	return nil
}

// PreviewResult says how many recent leads a trigger would have matched.
type PreviewResult struct {
	Checked int    `json:"checked"`
	Matched int    `json:"matched"`
	Samples []Lead `json:"samples"`
}

const previewCorpus = 100

// RecentLeads loads the preview corpus (database.Queries).
type RecentLeads interface {
	ListRecentScoredMentions(ctx context.Context, arg database.ListRecentScoredMentionsParams) ([]database.ListRecentScoredMentionsRow, error)
}

// Preview runs t over the workspace's latest scored posts (no side effects).
func Preview(ctx context.Context, q RecentLeads, wsID string, t Trigger) (PreviewResult, error) {
	rows, err := q.ListRecentScoredMentions(ctx, database.ListRecentScoredMentionsParams{WorkspaceID: wsID, Lim: previewCorpus})
	if err != nil {
		return PreviewResult{}, fmt.Errorf("load recent posts: %w", err)
	}
	res := PreviewResult{Checked: len(rows), Samples: []Lead{}}
	for _, r := range rows {
		l := Lead{
			MentionID: r.ID, Platform: r.Platform, Title: r.Title.String, Content: r.Content, URL: r.Url,
			Author: r.AuthorUsername.String, IntentTags: r.IntentTags, Score: float64(r.RelevanceScore.Float32),
		}
		if !t.Matches(l) {
			continue
		}
		res.Matched++
		if len(res.Samples) < 5 {
			l.Content = clip(l.Content, 240)
			res.Samples = append(res.Samples, l)
		}
	}
	return res, nil
}

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if s != "" {
		_ = u.Scan(s)
	}
	return u
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
