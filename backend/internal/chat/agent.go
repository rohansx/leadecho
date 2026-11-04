package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/apikey"
	"leadecho/internal/tools"
)

const (
	DefaultMaxSteps    = 6
	DefaultStepTimeout = 45 * time.Second
	maxResultChars     = 8000 // tool result JSON fed back to the model
)

// ToolCall statuses.
const (
	StatusDone      = "done"
	StatusError     = "error"
	StatusPending   = "pending" // write tool waiting for the user's confirm
	StatusRunning   = "running" // confirmed, executing
	StatusCancelled = "cancelled"
)

// Model is one chat completion; *llm.Router and StubModel satisfy it.
type Model interface {
	Chat(ctx context.Context, workspaceID string, msgs []ai.ChatMessage) (string, error)
}

// ToolSet is the tool registry; *tools.Registry satisfies it.
type ToolSet interface {
	List() []tools.Tool
	Call(ctx context.Context, workspaceID string, scopes []string, name string, args json.RawMessage) (any, error)
}

// ToolCall is one tool invocation recorded on an assistant message.
type ToolCall struct {
	ID      string          `json:"id"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	Status  string          `json:"status"`
	Confirm bool            `json:"confirm,omitempty"` // write tool gated on the user's confirm
	Error   string          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"` // confirmed write tools only
	Cards   *Cards          `json:"cards,omitempty"`
}

// Turn is the outcome of one user message.
type Turn struct {
	Answer string     `json:"answer"`
	Calls  []ToolCall `json:"tool_calls"`
	Steps  int        `json:"steps"`
}

type Agent struct {
	Model       Model
	Tools       ToolSet
	MaxSteps    int
	StepTimeout time.Duration
	Logger      zerolog.Logger
}

// chatScopes: a signed-in dashboard user may use every tool; write tools are
// gated by NeedsConfirm instead of scope.
var chatScopes = []string{apikey.ScopeRead, apikey.ScopeWrite}

// NeedsConfirm: write tools run only after the user confirms in the UI.
// draft_reply only creates an editable draft, so it runs directly.
func NeedsConfirm(t tools.Tool) bool {
	return t.Scope != apikey.ScopeRead && t.Name != "draft_reply"
}

// Run answers the last user message in history. emit streams progress events:
// tool_call, tool_result, confirm, answer.
func (a *Agent) Run(ctx context.Context, ws string, history []ai.ChatMessage, emit func(string, any)) (Turn, error) {
	list := a.Tools.List()
	byName := make(map[string]tools.Tool, len(list))
	for _, t := range list {
		byName[t.Name] = t
	}
	known := func(n string) bool { _, ok := byName[n]; return ok }
	msgs := append([]ai.ChatMessage{{Role: "system", Content: systemPrompt(list, time.Now())}}, history...)

	var turn Turn
	for turn.Steps < a.maxSteps() {
		turn.Steps++
		raw, err := a.step(ctx, ws, msgs)
		if err != nil {
			return turn, err
		}
		msgs = append(msgs, ai.ChatMessage{Role: "assistant", Content: raw})
		st, perr := ParseStep(raw, known)
		if perr != nil {
			msgs = append(msgs, ai.ChatMessage{Role: "user", Content: "PROTOCOL_ERROR: " + perr.Error() +
				`. Reply with exactly one JSON object: {"tool":"<name>","args":{...}} or {"answer":"<text>"}.`})
			continue
		}
		if st.Answer != "" {
			turn.Answer = st.Answer
			emit("answer", map[string]string{"content": turn.Answer})
			return turn, nil
		}
		call := ToolCall{ID: fmt.Sprintf("c%d", len(turn.Calls)+1), Tool: st.Tool, Args: st.Args}
		if NeedsConfirm(byName[st.Tool]) {
			call.Status, call.Confirm = StatusPending, true
			turn.Calls = append(turn.Calls, call)
			emit("confirm", call)
			turn.Answer = fmt.Sprintf("I'd like to run **%s**. Please confirm or cancel it below.", st.Tool)
			emit("answer", map[string]string{"content": turn.Answer})
			return turn, nil
		}
		emit("tool_call", call)
		call, result := a.runTool(ctx, ws, call)
		turn.Calls = append(turn.Calls, call)
		emit("tool_result", call)
		msgs = append(msgs, ai.ChatMessage{Role: "user", Content: toolResultMessage(call, result)})
	}
	turn.Answer = fmt.Sprintf("I couldn't finish within %d steps. Try a narrower question.", a.maxSteps())
	emit("answer", map[string]string{"content": turn.Answer})
	return turn, nil
}

func (a *Agent) maxSteps() int {
	if a.MaxSteps <= 0 {
		return DefaultMaxSteps
	}
	return a.MaxSteps
}

func (a *Agent) timeout() time.Duration {
	if a.StepTimeout <= 0 {
		return DefaultStepTimeout
	}
	return a.StepTimeout
}

func (a *Agent) step(ctx context.Context, ws string, msgs []ai.ChatMessage) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, a.timeout())
	defer cancel()
	raw, err := a.Model.Chat(sctx, ws, msgs)
	if err != nil {
		return "", fmt.Errorf("chat model: %w", err)
	}
	return raw, nil
}

func (a *Agent) runTool(ctx context.Context, ws string, call ToolCall) (ToolCall, any) {
	tctx, cancel := context.WithTimeout(ctx, a.timeout())
	defer cancel()
	res, err := a.Tools.Call(tctx, ws, chatScopes, call.Tool, call.Args)
	if err != nil {
		call.Status, call.Error = StatusError, publicError(err)
		if call.Error == internalErr {
			a.Logger.Warn().Err(err).Str("tool", call.Tool).Msg("chat: tool failed")
		}
		return call, nil
	}
	call.Status = StatusDone
	call.Cards = cardsFrom(res)
	return call, res
}

// Execute runs a write tool the user confirmed and records the outcome.
func Execute(ctx context.Context, ts ToolSet, ws string, call ToolCall) ToolCall {
	res, err := ts.Call(ctx, ws, chatScopes, call.Tool, call.Args)
	if err != nil {
		call.Status, call.Error = StatusError, publicError(err)
		return call
	}
	call.Status, call.Error = StatusDone, ""
	if b, err := json.Marshal(res); err == nil {
		call.Result = b
	}
	call.Cards = cardsFrom(res)
	return call
}

const internalErr = "internal error"

// publicError keeps caller-facing tool errors and hides everything else.
func publicError(err error) string {
	var te *tools.Error
	if errors.As(err, &te) {
		return te.Msg
	}
	if errors.Is(err, tools.ErrUnknownTool) {
		return "unknown tool"
	}
	return internalErr
}

func toolResultMessage(call ToolCall, result any) string {
	payload := map[string]any{"id": call.ID, "tool": call.Tool}
	if call.Status == StatusError {
		payload["error"] = call.Error
	} else {
		payload["result"] = result
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte(`{"error":"unencodable result"}`)
	}
	s := string(b)
	if len(s) > maxResultChars {
		s = s[:maxResultChars] + "…(truncated)"
	}
	return "TOOL_RESULT " + s
}

func systemPrompt(list []tools.Tool, now time.Time) string {
	var b strings.Builder
	b.WriteString(`You are "Ask LeadEcho", the assistant inside LeadEcho, a tool that finds buying-intent social mentions, drafts replies and attributes revenue.
Today is ` + now.UTC().Format("Monday 2006-01-02") + `.

PROTOCOL: every reply you send is EXACTLY ONE JSON object and nothing else:
  {"tool":"<tool name>","args":{...}}   to call a tool (you then receive TOOL_RESULT), or
  {"answer":"<markdown>"}               to answer the user and finish.
Rules: call tools to get facts; never invent mentions, leads or numbers. Use ids exactly as returned.
Keep answers short; the UI already shows mention and lead cards from tool results, so summarise and recommend instead of repeating every field.
Tools marked "needs user confirmation" are not run when you call them: the user sees a confirm button. Call them only when the user asked for that action.

TOOLS:
`)
	for _, t := range list {
		name := t.Name
		if NeedsConfirm(t) {
			name += " (needs user confirmation)"
		}
		fmt.Fprintf(&b, "- %s: %s\n  args schema: %s\n", name, t.Description, compactJSON(t.InputSchema))
	}
	return b.String()
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
