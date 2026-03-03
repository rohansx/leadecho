# LeadEcho - Workflow Engine Design

## Overview

The Workflow Engine automates mention-to-reply pipelines. Users define trigger conditions (platform, score, intent) and action chains (AI draft, notify, approval gate, post reply). The engine uses Redis Streams as a durable task queue with Go worker pools for execution.

```
Trigger (mention matches rules)
  → Action 1: AI Draft Reply
  → Action 2: Notify Slack
  → Action 3: Approval Gate (wait for human)
  → Action 4: Post Reply
```

**Design goals:** Durable execution (survives restarts), exactly-once semantics, human approval gates, per-tenant isolation, observable execution logs.

---

## Architecture

```
┌──────────────────────────────────────────────────┐
│                Workflow Engine                    │
│                                                  │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐ │
│  │ Trigger   │   │ Action   │   │ Approval     │ │
│  │ Listener  │──▶│ Executor │──▶│ Gate Manager │ │
│  └──────────┘   └──────────┘   └──────────────┘ │
│       ▲              │                │          │
│       │              ▼                ▼          │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐ │
│  │ Redis    │   │ Redis    │   │ PostgreSQL   │ │
│  │ Pub/Sub  │   │ Streams  │   │ (state)      │ │
│  └──────────┘   └──────────┘   └──────────────┘ │
└──────────────────────────────────────────────────┘
```

---

## Workflow Definition Model

### Database Schema

```sql
-- See 03-database-schema.md for full CREATE TABLE statements.
-- Key fields:
-- workflows: id, workspace_id, name, trigger_config (jsonb), action_chain (jsonb), is_active, created_at
-- workflow_executions: id, workflow_id, mention_id, status, current_step, step_results (jsonb), started_at, completed_at
```

### Go Types

```go
package workflow

type Workflow struct {
    ID            string         `json:"id"`
    WorkspaceID   string         `json:"workspace_id"`
    Name          string         `json:"name"`
    TriggerConfig TriggerConfig  `json:"trigger_config"`
    ActionChain   []ActionConfig `json:"action_chain"`
    IsActive      bool           `json:"is_active"`
    CreatedAt     time.Time      `json:"created_at"`
}

type TriggerConfig struct {
    Platforms   []string `json:"platforms"`             // ["reddit", "hackernews"]
    MinScore    float64  `json:"min_score"`             // 7.0
    IntentTypes []string `json:"intent_types"`          // ["buy_signal", "recommendation_ask"]
    Keywords    []string `json:"keywords,omitempty"`    // Optional keyword filter
    MaxAge      string   `json:"max_age,omitempty"`     // "6h" - ignore old mentions
}

type ActionConfig struct {
    Type   ActionType     `json:"type"`
    Config map[string]any `json:"config"`
}

type ActionType string

const (
    ActionAIDraft      ActionType = "ai_draft"
    ActionNotifySlack  ActionType = "notify_slack"
    ActionNotifyDiscord ActionType = "notify_discord"
    ActionApprovalGate ActionType = "approval_gate"
    ActionPostReply    ActionType = "post_reply"
    ActionCreateLead   ActionType = "create_lead"
    ActionWebhook      ActionType = "webhook"
    ActionDelay        ActionType = "delay"
    ActionCondition    ActionType = "condition"
    ActionTagMention   ActionType = "tag_mention"
)
```

### Example Workflow Definitions

```yaml
# Workflow 1: High-Intent Auto-Draft + Slack Notify
name: "High-Intent Reddit Auto-Draft"
trigger_config:
  platforms: ["reddit"]
  min_score: 7.0
  intent_types: ["buy_signal", "recommendation_ask"]
action_chain:
  - type: ai_draft
    config: {}
  - type: notify_slack
    config:
      channel: "#leads"
      mention_users: ["U1234"]
  - type: approval_gate
    config:
      timeout_hours: 24
      auto_action: "skip"  # skip if no response
  - type: post_reply
    config:
      variant: "recommended"  # use AI-recommended variant

# Workflow 2: All-Platform Monitor + Lead Creation
name: "High-Value Lead Capture"
trigger_config:
  platforms: ["reddit", "hackernews", "twitter", "linkedin"]
  min_score: 8.0
  intent_types: ["buy_signal"]
action_chain:
  - type: create_lead
    config:
      stage: "prospect"
  - type: ai_draft
    config: {}
  - type: notify_slack
    config:
      channel: "#high-intent"

# Workflow 3: Auto-Post for Low-Risk Platforms
name: "Reddit Auto-Engage"
trigger_config:
  platforms: ["reddit"]
  min_score: 8.5
  intent_types: ["recommendation_ask"]
action_chain:
  - type: ai_draft
    config: {}
  - type: delay
    config:
      minutes: 5
  - type: post_reply
    config:
      variant: "value_only"
      require_approval: false
```

---

## Trigger System

### Trigger Listener

```go
type TriggerListener struct {
    db     *database.Queries
    redis  *redis.Client
    engine *Engine
    logger zerolog.Logger
}

func (t *TriggerListener) Start(ctx context.Context) error {
    // Subscribe to scored mention events
    pubsub := t.redis.Subscribe(ctx, "mention.scored")
    defer pubsub.Close()

    ch := pubsub.Channel()
    for {
        select {
        case <-ctx.Done():
            return nil
        case msg := <-ch:
            var mention ScoredMention
            if err := json.Unmarshal([]byte(msg.Payload), &mention); err != nil {
                t.logger.Error().Err(err).Msg("unmarshal scored mention")
                continue
            }
            t.evaluateTriggers(ctx, mention)
        }
    }
}

func (t *TriggerListener) evaluateTriggers(ctx context.Context, mention ScoredMention) {
    // Get all active workflows for this workspace
    workflows, err := t.db.GetActiveWorkflows(ctx, mention.WorkspaceID)
    if err != nil {
        t.logger.Error().Err(err).Msg("get active workflows")
        return
    }

    for _, wf := range workflows {
        if t.matchesTrigger(mention, wf.TriggerConfig) {
            // Check dedup: don't trigger same workflow for same mention twice
            dedupKey := fmt.Sprintf("wf_dedup:%s:%s", wf.ID, mention.ID)
            if set, _ := t.redis.SetNX(ctx, dedupKey, "1", 24*time.Hour).Result(); !set {
                continue // Already triggered
            }

            t.engine.Enqueue(ctx, wf, mention)
        }
    }
}

func (t *TriggerListener) matchesTrigger(mention ScoredMention, config TriggerConfig) bool {
    // Platform check
    if len(config.Platforms) > 0 && !contains(config.Platforms, mention.Platform) {
        return false
    }

    // Score check
    if mention.RelevanceScore < config.MinScore {
        return false
    }

    // Intent check
    if len(config.IntentTypes) > 0 && !contains(config.IntentTypes, mention.Intent) {
        return false
    }

    // Age check
    if config.MaxAge != "" {
        maxAge, _ := time.ParseDuration(config.MaxAge)
        if time.Since(mention.PlatformTime) > maxAge {
            return false
        }
    }

    return true
}
```

---

## Action Types

| Action | Description | Input | Output |
|--------|-------------|-------|--------|
| `ai_draft` | Generate 3 reply variants via RAG pipeline | Mention context | Reply variants |
| `notify_slack` | Send mention summary to Slack channel | Mention + drafts | Message ID |
| `notify_discord` | Send via Discord webhook | Mention + drafts | Message ID |
| `approval_gate` | Wait for human approval | Drafts | Approved variant |
| `post_reply` | Post reply to platform | Approved content | Post confirmation |
| `create_lead` | Create lead in pipeline | Mention data | Lead ID |
| `webhook` | Call external URL | Configurable payload | Response status |
| `delay` | Wait N minutes before next step | Duration | - |
| `condition` | Branch based on mention properties | Condition expression | Branch choice |
| `tag_mention` | Add tags/labels to mention | Tag names | - |

### Action Executor

```go
type ActionExecutor interface {
    Execute(ctx context.Context, exec *Execution, step int) (*StepResult, error)
}

type StepResult struct {
    Status  string         `json:"status"` // "completed", "waiting", "failed", "skipped"
    Output  map[string]any `json:"output"`
    Error   string         `json:"error,omitempty"`
}

// Registry of action executors
type ActionRegistry struct {
    executors map[ActionType]ActionExecutor
}

func NewActionRegistry(deps Dependencies) *ActionRegistry {
    return &ActionRegistry{
        executors: map[ActionType]ActionExecutor{
            ActionAIDraft:       &AIDraftAction{generator: deps.ReplyGenerator},
            ActionNotifySlack:   &SlackNotifyAction{client: deps.SlackClient},
            ActionNotifyDiscord: &DiscordNotifyAction{client: deps.DiscordClient},
            ActionApprovalGate:  &ApprovalGateAction{db: deps.DB, notifier: deps.Notifier},
            ActionPostReply:     &PostReplyAction{adapters: deps.PlatformAdapters},
            ActionCreateLead:    &CreateLeadAction{db: deps.DB},
            ActionWebhook:       &WebhookAction{client: deps.HTTPClient},
            ActionDelay:         &DelayAction{},
            ActionCondition:     &ConditionAction{},
            ActionTagMention:    &TagMentionAction{db: deps.DB},
        },
    }
}
```

---

## Redis Streams Task Queue

### Enqueue Execution

```go
const workflowStream = "stream:workflow:executions"

func (e *Engine) Enqueue(ctx context.Context, wf Workflow, mention ScoredMention) error {
    // Create execution record in PostgreSQL
    execID := ulid.Make().String()
    exec := &Execution{
        ID:          execID,
        WorkflowID:  wf.ID,
        MentionID:   mention.ID,
        WorkspaceID: mention.WorkspaceID,
        Status:      "pending",
        CurrentStep: 0,
        StepResults: []StepResult{},
        Workflow:    wf,
        Mention:     mention,
    }

    if err := e.db.CreateWorkflowExecution(ctx, exec); err != nil {
        return fmt.Errorf("create execution: %w", err)
    }

    // Enqueue to Redis Stream
    payload, _ := json.Marshal(exec)
    _, err := e.redis.XAdd(ctx, &redis.XAddArgs{
        Stream: workflowStream,
        Values: map[string]any{
            "execution_id": execID,
            "payload":      string(payload),
        },
    }).Result()

    if err != nil {
        return fmt.Errorf("enqueue to stream: %w", err)
    }

    e.logger.Info().
        Str("execution_id", execID).
        Str("workflow", wf.Name).
        Str("mention", mention.ID).
        Msg("workflow execution enqueued")

    return nil
}
```

### Worker Pool

```go
type WorkerPool struct {
    redis    *redis.Client
    db       *database.Queries
    registry *ActionRegistry
    logger   zerolog.Logger
    group    string // Consumer group name
    workers  int
}

func NewWorkerPool(redis *redis.Client, db *database.Queries, registry *ActionRegistry, workers int) *WorkerPool {
    return &WorkerPool{
        redis:    redis,
        db:       db,
        registry: registry,
        group:    "workflow-workers",
        workers:  workers,
        logger:   zerolog.New(os.Stdout).With().Str("component", "workflow-worker").Logger(),
    }
}

func (wp *WorkerPool) Start(ctx context.Context) error {
    // Create consumer group (idempotent)
    wp.redis.XGroupCreateMkStream(ctx, workflowStream, wp.group, "0")

    g, ctx := errgroup.WithContext(ctx)

    for i := 0; i < wp.workers; i++ {
        workerID := fmt.Sprintf("worker-%d", i)
        g.Go(func() error {
            return wp.runWorker(ctx, workerID)
        })
    }

    // Claim stale messages (from crashed workers)
    g.Go(func() error {
        return wp.reclaimStale(ctx)
    })

    return g.Wait()
}

func (wp *WorkerPool) runWorker(ctx context.Context, workerID string) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        default:
        }

        // Block-read from stream with 5s timeout
        results, err := wp.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    wp.group,
            Consumer: workerID,
            Streams:  []string{workflowStream, ">"},
            Count:    1,
            Block:    5 * time.Second,
        }).Result()

        if err != nil {
            if errors.Is(err, redis.Nil) {
                continue // No messages, loop again
            }
            wp.logger.Error().Err(err).Msg("read from stream")
            time.Sleep(time.Second)
            continue
        }

        for _, stream := range results {
            for _, msg := range stream.Messages {
                wp.processMessage(ctx, workerID, msg)
            }
        }
    }
}

func (wp *WorkerPool) processMessage(ctx context.Context, workerID string, msg redis.XMessage) {
    var exec Execution
    if err := json.Unmarshal([]byte(msg.Values["payload"].(string)), &exec); err != nil {
        wp.logger.Error().Err(err).Msg("unmarshal execution")
        wp.redis.XAck(ctx, workflowStream, wp.group, msg.ID)
        return
    }

    wp.logger.Info().
        Str("execution_id", exec.ID).
        Str("worker", workerID).
        Int("step", exec.CurrentStep).
        Msg("processing workflow step")

    // Execute current step
    result := wp.executeStep(ctx, &exec)

    // Update execution state
    exec.StepResults = append(exec.StepResults, *result)

    switch result.Status {
    case "completed":
        exec.CurrentStep++
        if exec.CurrentStep >= len(exec.Workflow.ActionChain) {
            // All steps complete
            exec.Status = "completed"
            exec.CompletedAt = timePtr(time.Now())
            wp.db.UpdateWorkflowExecution(ctx, &exec)
            wp.redis.XAck(ctx, workflowStream, wp.group, msg.ID)

            wp.logger.Info().Str("execution_id", exec.ID).Msg("workflow completed")
        } else {
            // More steps - re-enqueue
            wp.db.UpdateWorkflowExecution(ctx, &exec)
            wp.redis.XAck(ctx, workflowStream, wp.group, msg.ID)
            wp.reEnqueue(ctx, &exec)
        }

    case "waiting":
        // Approval gate - update state, ACK message (gate manager will re-enqueue)
        exec.Status = "waiting_approval"
        wp.db.UpdateWorkflowExecution(ctx, &exec)
        wp.redis.XAck(ctx, workflowStream, wp.group, msg.ID)

    case "failed":
        exec.Status = "failed"
        exec.CompletedAt = timePtr(time.Now())
        wp.db.UpdateWorkflowExecution(ctx, &exec)
        wp.redis.XAck(ctx, workflowStream, wp.group, msg.ID)

        wp.logger.Error().
            Str("execution_id", exec.ID).
            Str("error", result.Error).
            Msg("workflow step failed")
    }
}

func (wp *WorkerPool) executeStep(ctx context.Context, exec *Execution) *StepResult {
    action := exec.Workflow.ActionChain[exec.CurrentStep]
    executor, ok := wp.registry.executors[action.Type]
    if !ok {
        return &StepResult{Status: "failed", Error: fmt.Sprintf("unknown action: %s", action.Type)}
    }

    result, err := executor.Execute(ctx, exec, exec.CurrentStep)
    if err != nil {
        return &StepResult{Status: "failed", Error: err.Error()}
    }
    return result
}
```

### Stale Message Reclamation

```go
func (wp *WorkerPool) reclaimStale(ctx context.Context) error {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            // Claim messages idle for > 60 seconds (likely from crashed workers)
            pending, _ := wp.redis.XPendingExt(ctx, &redis.XPendingExtArgs{
                Stream: workflowStream,
                Group:  wp.group,
                Start:  "-",
                End:    "+",
                Count:  10,
            }).Result()

            for _, p := range pending {
                if p.Idle > 60*time.Second {
                    wp.redis.XClaim(ctx, &redis.XClaimArgs{
                        Stream:   workflowStream,
                        Group:    wp.group,
                        Consumer: "reclaimer",
                        MinIdle:  60 * time.Second,
                        Messages: []string{p.ID},
                    })
                    wp.logger.Warn().Str("message_id", p.ID).Msg("reclaimed stale message")
                }
            }
        }
    }
}
```

---

## Approval Gate

### State Machine

```
┌─────────┐   trigger   ┌─────────┐   approve   ┌──────────┐
│ Pending  │────────────▶│ Waiting │────────────▶│ Approved │
└─────────┘              └─────────┘              └──────────┘
                              │                        │
                              │ reject                 │
                              ▼                        ▼
                         ┌─────────┐            ┌──────────┐
                         │Rejected │            │ Continue  │
                         └─────────┘            │ Chain     │
                              │                 └──────────┘
                              │ timeout
                              ▼
                         ┌─────────┐
                         │ Skipped │ (auto_action: "skip")
                         └─────────┘
```

### Approval Gate Action

```go
type ApprovalGateAction struct {
    db       *database.Queries
    notifier Notifier
}

func (a *ApprovalGateAction) Execute(ctx context.Context, exec *Execution, step int) (*StepResult, error) {
    config := exec.Workflow.ActionChain[step].Config
    timeoutHours := getFloat64(config, "timeout_hours", 24)

    // Store approval request
    approval := &ApprovalRequest{
        ID:          ulid.Make().String(),
        ExecutionID: exec.ID,
        WorkspaceID: exec.WorkspaceID,
        MentionID:   exec.MentionID,
        Step:        step,
        Status:      "pending",
        ExpiresAt:   time.Now().Add(time.Duration(timeoutHours) * time.Hour),
        AutoAction:  getString(config, "auto_action", "skip"),
    }

    if err := a.db.CreateApprovalRequest(ctx, approval); err != nil {
        return nil, fmt.Errorf("create approval: %w", err)
    }

    // Notify via dashboard SSE
    a.notifier.Notify(ctx, exec.WorkspaceID, NotificationEvent{
        Type:    "approval.requested",
        Title:   "Reply needs approval",
        Message: fmt.Sprintf("Workflow '%s' generated a reply for review", exec.Workflow.Name),
        Data: map[string]any{
            "execution_id": exec.ID,
            "mention_id":   exec.MentionID,
            "approval_id":  approval.ID,
        },
    })

    return &StepResult{
        Status: "waiting",
        Output: map[string]any{
            "approval_id": approval.ID,
            "expires_at":  approval.ExpiresAt.Format(time.RFC3339),
        },
    }, nil
}
```

### Approval Resolver

```go
func (e *Engine) ResolveApproval(ctx context.Context, approvalID string, decision string, userID string) error {
    approval, err := e.db.GetApprovalRequest(ctx, approvalID)
    if err != nil {
        return fmt.Errorf("get approval: %w", err)
    }

    if approval.Status != "pending" {
        return fmt.Errorf("approval already resolved: %s", approval.Status)
    }

    // Update approval
    approval.Status = decision // "approved" or "rejected"
    approval.ResolvedBy = userID
    approval.ResolvedAt = timePtr(time.Now())
    if err := e.db.UpdateApprovalRequest(ctx, approval); err != nil {
        return err
    }

    // Get the execution
    exec, err := e.db.GetWorkflowExecution(ctx, approval.ExecutionID)
    if err != nil {
        return err
    }

    if decision == "approved" {
        // Advance to next step and re-enqueue
        exec.CurrentStep++
        exec.Status = "running"
        e.db.UpdateWorkflowExecution(ctx, exec)
        e.reEnqueue(ctx, exec)
    } else {
        // Rejected - mark execution as cancelled
        exec.Status = "cancelled"
        exec.CompletedAt = timePtr(time.Now())
        e.db.UpdateWorkflowExecution(ctx, exec)
    }

    return nil
}
```

### Expiration Sweeper

```go
func (e *Engine) StartExpirationSweeper(ctx context.Context) error {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            expired, _ := e.db.GetExpiredApprovals(ctx, time.Now())
            for _, approval := range expired {
                switch approval.AutoAction {
                case "skip":
                    // Skip posting, mark as completed
                    e.ResolveApproval(ctx, approval.ID, "skipped", "system")
                case "post":
                    // Auto-approve and post
                    e.ResolveApproval(ctx, approval.ID, "approved", "system")
                }
                e.logger.Info().
                    Str("approval_id", approval.ID).
                    Str("auto_action", approval.AutoAction).
                    Msg("approval expired, auto-action taken")
            }
        }
    }
}
```

---

## Execution Logging

```go
type ExecutionLog struct {
    ExecutionID string         `json:"execution_id"`
    Step        int            `json:"step"`
    Action      ActionType     `json:"action"`
    Status      string         `json:"status"`
    Input       map[string]any `json:"input,omitempty"`
    Output      map[string]any `json:"output,omitempty"`
    Error       string         `json:"error,omitempty"`
    Duration    time.Duration  `json:"duration_ms"`
    Timestamp   time.Time      `json:"timestamp"`
}

// Query execution history
// -- name: GetExecutionHistory :many
// SELECT * FROM workflow_executions
// WHERE workflow_id = @workflow_id
// ORDER BY started_at DESC
// LIMIT @limit OFFSET @offset;

// -- name: GetExecutionsByStatus :many
// SELECT * FROM workflow_executions
// WHERE workspace_id = @workspace_id AND status = @status
// ORDER BY started_at DESC;
```

---

## Slack Integration

### Notification Action

```go
type SlackNotifyAction struct {
    client *slack.Client
}

func (s *SlackNotifyAction) Execute(ctx context.Context, exec *Execution, step int) (*StepResult, error) {
    config := exec.Workflow.ActionChain[step].Config
    channel := getString(config, "channel", "#leads")

    // Build rich notification
    blocks := []slack.Block{
        slack.NewHeaderBlock(
            slack.NewTextBlockObject("plain_text", "New Lead Signal Detected", false, false),
        ),
        slack.NewSectionBlock(
            slack.NewTextBlockObject("mrkdwn", fmt.Sprintf(
                "*Platform:* %s\n*Score:* %.1f/10\n*Intent:* %s\n*Author:* %s",
                exec.Mention.Platform,
                exec.Mention.RelevanceScore,
                exec.Mention.Intent,
                exec.Mention.Author.Username,
            )),
            nil, nil,
        ),
        slack.NewSectionBlock(
            slack.NewTextBlockObject("mrkdwn", fmt.Sprintf(
                ">>> %s", truncate(exec.Mention.Content, 300),
            )),
            nil, nil,
        ),
        slack.NewActionBlock("approve_actions",
            slack.NewButtonBlockElement("approve", exec.ID,
                slack.NewTextBlockObject("plain_text", "Approve Reply", false, false),
            ).WithStyle(slack.StylePrimary),
            slack.NewButtonBlockElement("reject", exec.ID,
                slack.NewTextBlockObject("plain_text", "Reject", false, false),
            ).WithStyle(slack.StyleDanger),
            slack.NewButtonBlockElement("view", exec.Mention.URL,
                slack.NewTextBlockObject("plain_text", "View Post", false, false),
            ),
        ),
    }

    _, _, err := s.client.PostMessageContext(ctx, channel,
        slack.MsgOptionBlocks(blocks...),
    )
    if err != nil {
        return nil, fmt.Errorf("slack post: %w", err)
    }

    return &StepResult{
        Status: "completed",
        Output: map[string]any{"channel": channel},
    }, nil
}
```

### Slack Interactive Callback

```go
// Handle Slack button clicks for approval/rejection
func (h *WebhookHandler) HandleSlackInteraction(w http.ResponseWriter, r *http.Request) {
    payload := r.FormValue("payload")
    var interaction slack.InteractionCallback
    if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
        http.Error(w, "bad payload", http.StatusBadRequest)
        return
    }

    // Verify Slack signing secret
    if !verifySlackSignature(r, h.slackSigningSecret) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    for _, action := range interaction.ActionCallback.BlockActions {
        switch action.ActionID {
        case "approve":
            h.engine.ResolveApproval(r.Context(), action.Value, "approved", interaction.User.ID)
            respondToSlack(w, "Reply approved and queued for posting.")
        case "reject":
            h.engine.ResolveApproval(r.Context(), action.Value, "rejected", interaction.User.ID)
            respondToSlack(w, "Reply rejected.")
        }
    }
}
```

---

## Discord Integration

### Webhook Notification

```go
type DiscordNotifyAction struct {
    client *http.Client
}

func (d *DiscordNotifyAction) Execute(ctx context.Context, exec *Execution, step int) (*StepResult, error) {
    config := exec.Workflow.ActionChain[step].Config
    webhookURL := getString(config, "webhook_url", "")

    embed := DiscordEmbed{
        Title:       "New Lead Signal Detected",
        Description: truncate(exec.Mention.Content, 300),
        Color:       getColorForScore(exec.Mention.RelevanceScore),
        Fields: []DiscordField{
            {Name: "Platform", Value: exec.Mention.Platform, Inline: true},
            {Name: "Score", Value: fmt.Sprintf("%.1f/10", exec.Mention.RelevanceScore), Inline: true},
            {Name: "Intent", Value: exec.Mention.Intent, Inline: true},
            {Name: "Author", Value: exec.Mention.Author.Username, Inline: true},
        },
        URL:       exec.Mention.URL,
        Timestamp: exec.Mention.PlatformTime.Format(time.RFC3339),
    }

    payload := DiscordWebhookPayload{
        Embeds: []DiscordEmbed{embed},
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := d.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("discord webhook: %w", err)
    }
    defer resp.Body.Close()

    return &StepResult{Status: "completed", Output: map[string]any{"status": resp.StatusCode}}, nil
}
```

---

## Error Handling & Resilience

### Per-Action Circuit Breaker

```go
type ResilientExecutor struct {
    inner    ActionExecutor
    breaker  *CircuitBreaker
    maxRetry int
}

func (r *ResilientExecutor) Execute(ctx context.Context, exec *Execution, step int) (*StepResult, error) {
    if !r.breaker.Allow() {
        return &StepResult{
            Status: "failed",
            Error:  "circuit breaker open: too many recent failures",
        }, nil
    }

    var lastErr error
    for attempt := 0; attempt <= r.maxRetry; attempt++ {
        if attempt > 0 {
            time.Sleep(backoff(attempt))
        }

        result, err := r.inner.Execute(ctx, exec, step)
        if err == nil && result.Status != "failed" {
            r.breaker.RecordSuccess()
            return result, nil
        }

        lastErr = err
        if err != nil {
            r.breaker.RecordFailure()
        }
    }

    return &StepResult{Status: "failed", Error: lastErr.Error()}, nil
}
```

### Dead Letter Queue

```go
const dlqStream = "stream:workflow:dlq"

func (wp *WorkerPool) sendToDLQ(ctx context.Context, msg redis.XMessage, err error) {
    wp.redis.XAdd(ctx, &redis.XAddArgs{
        Stream: dlqStream,
        Values: map[string]any{
            "original_id": msg.ID,
            "payload":     msg.Values["payload"],
            "error":       err.Error(),
            "failed_at":   time.Now().Format(time.RFC3339),
        },
    })
}
```

---

## Configuration & Limits

### Per-Tier Limits

| Tier | Active Workflows | Actions/Chain | Executions/Day |
|------|-----------------|---------------|----------------|
| Starter | 1 | 3 | 10 |
| Solo | 3 | 5 | 50 |
| Growth | 10 | 8 | 200 |
| Scale | 25 | 12 | Unlimited |

### Engine Startup Wiring

```go
func SetupWorkflowEngine(db *database.Queries, rdb *redis.Client, deps Dependencies) (*Engine, error) {
    registry := NewActionRegistry(deps)
    engine := &Engine{
        db:       db,
        redis:    rdb,
        registry: registry,
    }

    pool := NewWorkerPool(rdb, db, registry, 4) // 4 worker goroutines

    g, ctx := errgroup.WithContext(context.Background())

    // Trigger listener
    listener := &TriggerListener{db: db, redis: rdb, engine: engine}
    g.Go(func() error { return listener.Start(ctx) })

    // Worker pool
    g.Go(func() error { return pool.Start(ctx) })

    // Approval expiration sweeper
    g.Go(func() error { return engine.StartExpirationSweeper(ctx) })

    return engine, nil
}
```

---

## Workflow CRUD API

```go
// POST /api/v1/workflows
func (h *Handler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req CreateWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
        return
    }

    // Validate action chain
    for _, action := range req.ActionChain {
        if _, ok := h.registry.executors[action.Type]; !ok {
            respondError(w, http.StatusBadRequest, "INVALID_ACTION", fmt.Sprintf("unknown action: %s", action.Type))
            return
        }
    }

    // Check tier limits
    workspaceID := getWorkspaceID(r)
    count, _ := h.db.CountActiveWorkflows(r.Context(), workspaceID)
    limit := h.tierLimits.MaxWorkflows(workspaceID)
    if count >= limit {
        respondError(w, http.StatusForbidden, "LIMIT_EXCEEDED", "workflow limit reached for your plan")
        return
    }

    wf := &Workflow{
        ID:            ulid.Make().String(),
        WorkspaceID:   workspaceID,
        Name:          req.Name,
        TriggerConfig: req.TriggerConfig,
        ActionChain:   req.ActionChain,
        IsActive:      true,
    }

    if err := h.db.CreateWorkflow(r.Context(), wf); err != nil {
        respondError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
        return
    }

    respondJSON(w, http.StatusCreated, wf)
}
```
