package chat

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/apikey"
	"leadecho/internal/tools"
)

// scriptModel replies with the scripted outputs in order (the last repeats)
// and records every prompt it saw.
type scriptModel struct {
	replies []string
	block   bool // wait for ctx cancellation instead of answering
	mu      sync.Mutex
	seen    [][]ai.ChatMessage
}

func (m *scriptModel) Chat(ctx context.Context, ws string, msgs []ai.ChatMessage) (string, error) {
	m.mu.Lock()
	m.seen = append(m.seen, slices.Clone(msgs))
	i := min(len(m.seen)-1, len(m.replies)-1)
	m.mu.Unlock()
	if m.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return m.replies[i], nil
}

type fakeCall struct {
	ws, name string
	scopes   []string
	args     string
}

// fakeTools mimics tools.Registry with canned results.
type fakeTools struct {
	results map[string]any
	errs    map[string]error
	calls   []fakeCall
}

func (f *fakeTools) List() []tools.Tool {
	return []tools.Tool{
		{Name: "search_mentions", Description: "search", Scope: apikey.ScopeRead, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "list_leads", Description: "leads", Scope: apikey.ScopeRead, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "attribution_report", Description: "attr", Scope: apikey.ScopeRead, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "draft_reply", Description: "draft", Scope: apikey.ScopeWrite, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "set_stage", Description: "stage", Scope: apikey.ScopeWrite, InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "queue_reply", Description: "queue", Scope: apikey.ScopeWrite, InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
}

func (f *fakeTools) Call(ctx context.Context, ws string, scopes []string, name string, args json.RawMessage) (any, error) {
	f.calls = append(f.calls, fakeCall{ws: ws, name: name, scopes: scopes, args: string(args)})
	if err := f.errs[name]; err != nil {
		return nil, err
	}
	return f.results[name], nil
}

type recorder struct{ events []string }

func (r *recorder) emit(ev string, _ any) { r.events = append(r.events, ev) }

func sampleMentions() map[string]any {
	title := "Looking for a Mention alternative"
	return map[string]any{"mentions": []tools.MentionView{{ID: "m1", Platform: "reddit", URL: "https://r/1", Title: &title, Content: "help", Status: "new"}}, "count": 1}
}

func TestAgentRun(t *testing.T) {
	tests := []struct {
		name        string
		replies     []string
		errs        map[string]error
		wantAnswer  string
		wantCalls   []string // tools executed
		wantStatus  []string // ToolCall statuses on the turn
		wantEvents  []string
		wantInModel string // substring of the last prompt the model saw
		wantCards   int
	}{
		{
			name:       "read tool then answer",
			replies:    []string{`{"tool":"search_mentions","args":{"tier":"leads_ready"}}`, `{"answer":"One buyer on reddit."}`},
			wantAnswer: "One buyer on reddit.", wantCalls: []string{"search_mentions"}, wantStatus: []string{"done"},
			wantEvents:  []string{"tool_call", "tool_result", "answer"},
			wantInModel: "Looking for a Mention alternative", wantCards: 1,
		},
		{
			name:       "write tool is not executed but returned for confirmation",
			replies:    []string{`{"tool":"set_stage","args":{"lead_id":"l1","stage":"qualified"}}`},
			wantAnswer: "confirm", wantCalls: nil, wantStatus: []string{"pending"},
			wantEvents: []string{"confirm", "answer"},
		},
		{
			name:       "draft_reply runs without confirmation",
			replies:    []string{`{"tool":"draft_reply","args":{"mention_id":"m1"}}`, `{"answer":"Drafted."}`},
			wantAnswer: "Drafted.", wantCalls: []string{"draft_reply"}, wantStatus: []string{"done"},
			wantEvents: []string{"tool_call", "tool_result", "answer"},
		},
		{
			name:       "protocol error is fed back and the model recovers",
			replies:    []string{`Sure, here you go`, `{"answer":"Fixed."}`},
			wantAnswer: "Fixed.", wantEvents: []string{"answer"}, wantInModel: "PROTOCOL_ERROR",
		},
		{
			name:       "tool user error is shown to the model",
			replies:    []string{`{"tool":"list_leads","args":{"stage":"x"}}`, `{"answer":"Bad stage."}`},
			errs:       map[string]error{"list_leads": &tools.Error{Msg: "invalid stage \"x\""}},
			wantAnswer: "Bad stage.", wantCalls: []string{"list_leads"}, wantStatus: []string{"error"},
			wantEvents: []string{"tool_call", "tool_result", "answer"}, wantInModel: `invalid stage`,
		},
		{
			name:       "internal tool error is masked",
			replies:    []string{`{"tool":"list_leads"}`, `{"answer":"Sorry."}`},
			errs:       map[string]error{"list_leads": errors.New("pq: password=hunter2")},
			wantAnswer: "Sorry.", wantCalls: []string{"list_leads"}, wantStatus: []string{"error"},
			wantEvents: []string{"tool_call", "tool_result", "answer"}, wantInModel: "internal error",
		},
		{
			name:       "step budget is enforced",
			replies:    []string{`{"tool":"list_leads"}`},
			wantAnswer: "couldn't finish", wantCalls: []string{"list_leads", "list_leads", "list_leads"},
			wantStatus: []string{"done", "done", "done"},
			wantEvents: []string{"tool_call", "tool_result", "tool_call", "tool_result", "tool_call", "tool_result", "answer"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &scriptModel{replies: tc.replies}
			ft := &fakeTools{results: map[string]any{"search_mentions": sampleMentions(), "list_leads": map[string]any{"leads": []any{}}}, errs: tc.errs}
			a := &Agent{Model: m, Tools: ft, MaxSteps: 3, StepTimeout: time.Second}
			rec := &recorder{}
			turn, err := a.Run(context.Background(), "ws-1", []ai.ChatMessage{{Role: "user", Content: "best leads?"}}, rec.emit)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(turn.Answer, tc.wantAnswer) {
				t.Errorf("answer = %q, want containing %q", turn.Answer, tc.wantAnswer)
			}
			var names []string
			for _, c := range ft.calls {
				names = append(names, c.name)
				if c.ws != "ws-1" {
					t.Errorf("tool got workspace %q", c.ws)
				}
			}
			if !slices.Equal(names, tc.wantCalls) {
				t.Errorf("executed %v, want %v", names, tc.wantCalls)
			}
			var statuses []string
			cards := 0
			for _, c := range turn.Calls {
				statuses = append(statuses, c.Status)
				if c.Cards != nil {
					cards += len(c.Cards.Mentions) + len(c.Cards.Leads)
				}
			}
			if !slices.Equal(statuses, tc.wantStatus) {
				t.Errorf("statuses %v, want %v", statuses, tc.wantStatus)
			}
			if !slices.Equal(rec.events, tc.wantEvents) {
				t.Errorf("events %v, want %v", rec.events, tc.wantEvents)
			}
			if cards != tc.wantCards {
				t.Errorf("cards = %d, want %d", cards, tc.wantCards)
			}
			if tc.wantInModel != "" {
				last := m.seen[len(m.seen)-1]
				if !strings.Contains(last[len(last)-1].Content, tc.wantInModel) {
					t.Errorf("model's last message %q lacks %q", last[len(last)-1].Content, tc.wantInModel)
				}
			}
			if strings.Contains(strings.Join(lastContents(m), "\n"), "hunter2") {
				t.Error("internal error leaked to the model")
			}
		})
	}
}

func lastContents(m *scriptModel) []string {
	var out []string
	for _, p := range m.seen {
		for _, msg := range p {
			out = append(out, msg.Content)
		}
	}
	return out
}

func TestAgentSystemPromptListsToolsAndConfirmation(t *testing.T) {
	m := &scriptModel{replies: []string{`{"answer":"hi"}`}}
	a := &Agent{Model: m, Tools: &fakeTools{}, MaxSteps: 2, StepTimeout: time.Second}
	if _, err := a.Run(context.Background(), "ws", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string, any) {}); err != nil {
		t.Fatal(err)
	}
	sys := m.seen[0][0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "search_mentions") || !strings.Contains(sys.Content, "set_stage (needs user confirmation)") {
		t.Fatalf("system prompt: %s", sys.Content)
	}
	if strings.Contains(sys.Content, "draft_reply (needs user confirmation)") {
		t.Fatal("draft_reply should not need confirmation")
	}
}

func TestAgentStepTimeout(t *testing.T) {
	a := &Agent{Model: &scriptModel{replies: []string{""}, block: true}, Tools: &fakeTools{}, MaxSteps: 3, StepTimeout: 20 * time.Millisecond}
	start := time.Now()
	_, err := a.Run(context.Background(), "ws", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string, any) {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("step timeout not applied")
	}
}

func TestExecuteConfirmed(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantErr    string
	}{
		{"runs with write scope", nil, "done", ""},
		{"user error is kept", &tools.Error{Msg: "lead not found"}, "error", "lead not found"},
		{"internal error is masked", errors.New("db down"), "error", "internal error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeTools{results: map[string]any{"set_stage": map[string]any{"ok": true}}, errs: map[string]error{"set_stage": tc.err}}
			call := ToolCall{ID: "c1", Tool: "set_stage", Args: json.RawMessage(`{"lead_id":"l1","stage":"qualified"}`), Status: StatusPending}
			got := Execute(context.Background(), ft, "ws-9", call)
			if got.Status != tc.wantStatus || got.Error != tc.wantErr {
				t.Fatalf("got %+v", got)
			}
			if len(ft.calls) != 1 || ft.calls[0].ws != "ws-9" || !slices.Contains(ft.calls[0].scopes, apikey.ScopeWrite) || ft.calls[0].args != string(call.Args) {
				t.Fatalf("calls %+v", ft.calls)
			}
		})
	}
}
