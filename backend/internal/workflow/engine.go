package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
)

type Engine struct {
	q        *database.Queries
	publisher *publishers.Publisher
	logger   zerolog.Logger
}

func NewEngine(q *database.Queries, publisher *publishers.Publisher, logger zerolog.Logger) *Engine {
	return &Engine{
		q:        q,
		publisher: publisher,
		logger:   logger.With().Str("component", "workflow_engine").Logger(),
	}
}

func (e *Engine) EvaluateAndTrigger(ctx context.Context, payload events.WorkflowTriggerRequestedPayload) error {
	workflows, err := e.q.ListActiveWorkflowsByWorkspace(ctx, payload.WorkspaceID)
	if err != nil {
		return fmt.Errorf("list workflows: %w", err)
	}

	for _, wf := range workflows {
		var trigger TriggerConfig
		if err := json.Unmarshal(wf.TriggerConfig, &trigger); err != nil {
			e.logger.Error().Err(err).Str("workflow_id", wf.ID).Msg("invalid trigger_config")
			continue
		}
		if !matchesTrigger(payload, trigger) {
			continue
		}

		exists, err := e.q.HasWorkflowExecutionForMention(ctx, database.HasWorkflowExecutionForMentionParams{
			WorkflowID: wf.ID,
			MentionID:  pgUUID(payload.MentionID),
		})
		if err != nil {
			return fmt.Errorf("dedup check: %w", err)
		}
		if exists {
			continue
		}

		if err := e.startExecution(ctx, wf, payload); err != nil {
			e.logger.Error().Err(err).Str("workflow_id", wf.ID).Str("mention_id", payload.MentionID).Msg("workflow execution failed")
		}
	}
	return nil
}

func matchesTrigger(payload events.WorkflowTriggerRequestedPayload, trigger TriggerConfig) bool {
	if trigger.MinScore > 0 && float64(payload.RelevanceScore) < trigger.MinScore {
		return false
	}
	if len(trigger.Platforms) > 0 && !containsString(trigger.Platforms, payload.Platform) {
		return false
	}
	if len(trigger.IntentTypes) > 0 {
		if payload.Intent == "" || !containsString(trigger.IntentTypes, payload.Intent) {
			return false
		}
	}
	if len(trigger.Keywords) > 0 {
		lower := strings.ToLower(payload.Title + " " + payload.Content)
		matched := false
		for _, kw := range trigger.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (e *Engine) startExecution(ctx context.Context, wf database.Workflow, payload events.WorkflowTriggerRequestedPayload) error {
	var actions []ActionConfig
	if err := json.Unmarshal(wf.ActionChain, &actions); err != nil {
		return fmt.Errorf("parse action_chain: %w", err)
	}

	steps := make([]StepResult, 0, len(actions))
	exec, err := e.q.CreateWorkflowExecution(ctx, database.CreateWorkflowExecutionParams{
		WorkflowID:  wf.ID,
		MentionID:   pgUUID(payload.MentionID),
		Status:      database.ExecutionStatusRunning,
		Steps:       mustJSON(steps),
		CurrentStep: 0,
	})
	if err != nil {
		return fmt.Errorf("create execution: %w", err)
	}

	if _, err := e.q.IncrementWorkflowTriggerCount(ctx, wf.ID); err != nil {
		e.logger.Error().Err(err).Str("workflow_id", wf.ID).Msg("increment trigger count")
	}

	currentStep := int32(0)
	status := database.ExecutionStatusRunning
	var lastErr error

	for i, action := range actions {
		stepResult := StepResult{Step: i, Type: action.Type, Status: "running"}
		switch action.Type {
		case ActionAIDraft:
			if e.publisher == nil {
				stepResult.Status = "skipped"
				stepResult.Detail = "event publisher not configured"
			} else {
				env, err := events.NewEnvelope(
					events.EventTypeReplyDraftRequested,
					events.AggregateTypeReply,
					payload.MentionID,
					"workflow_engine",
					payload.WorkspaceID,
					fmt.Sprintf("%s:%s:%s", events.EventTypeReplyDraftRequested, exec.ID, payload.MentionID),
					events.ReplyDraftRequestedPayload{
						MentionID:           payload.MentionID,
						WorkspaceID:         payload.WorkspaceID,
						WorkflowExecutionID: exec.ID,
						Source:              "workflow",
					},
				)
				if err != nil {
					stepResult.Status = "failed"
					stepResult.Detail = err.Error()
					lastErr = err
				} else if _, err := e.publisher.Publish(ctx, env); err != nil {
					stepResult.Status = "failed"
					stepResult.Detail = err.Error()
					lastErr = err
				} else {
					stepResult.Status = "completed"
					stepResult.Detail = "reply.draft_requested published"
				}
			}
		case ActionCreateLead:
			stepResult.Status = "deferred"
			stepResult.Detail = "handled by mention qualifier pipeline"
		case ActionApprovalGate, ActionPostReply, ActionNotifySlack, ActionNotifyDiscord, ActionWebhook, ActionDelay, ActionCondition, ActionTagMention:
			stepResult.Status = "deferred"
			stepResult.Detail = "not implemented in v1 executor"
		default:
			stepResult.Status = "skipped"
			stepResult.Detail = "unknown action type"
		}

		steps = append(steps, stepResult)
		currentStep = int32(i + 1)

		if stepResult.Status == "failed" {
			status = database.ExecutionStatusFailed
			break
		}
	}

	if status == database.ExecutionStatusRunning {
		status = database.ExecutionStatusCompleted
	}

	var completedAt pgtype.Timestamptz
	if status == database.ExecutionStatusCompleted {
		completedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	errMsg := pgtype.Text{}
	if lastErr != nil {
		errMsg = pgtype.Text{String: lastErr.Error(), Valid: true}
	}

	_, err = e.q.UpdateWorkflowExecution(ctx, database.UpdateWorkflowExecutionParams{
		ID:           exec.ID,
		Status:       status,
		CurrentStep:  currentStep,
		Steps:        mustJSON(steps),
		ErrorMessage: errMsg,
		CompletedAt:  completedAt,
	})
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}

	e.logger.Info().
		Str("workflow_id", wf.ID).
		Str("execution_id", exec.ID).
		Str("mention_id", payload.MentionID).
		Str("status", string(status)).
		Msg("workflow execution finished")
	return lastErr
}

func containsString(items []string, v string) bool {
	for _, item := range items {
		if item == v {
			return true
		}
	}
	return false
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func pgUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if s == "" {
		return u
	}
	_ = u.Scan(s)
	return u
}
