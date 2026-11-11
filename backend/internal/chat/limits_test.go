package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
)

// A post or web page the assistant reads can try to make it draft replies in
// a loop. Each draft costs credits and runs without a confirm, so one
// question may run draft_reply once.
func TestAgentCapsCostlyToolsPerTurn(t *testing.T) {
	model := &scriptModel{replies: []string{
		`{"tool":"draft_reply","args":{"mention_id":"m1"}}`,
		`{"tool":"draft_reply","args":{"mention_id":"m2"}}`,
		`{"tool":"draft_reply","args":{"mention_id":"m3"}}`,
		`{"answer":"Drafted one reply."}`,
	}}
	ft := &fakeTools{results: map[string]any{"draft_reply": map[string]any{"ok": true}}}
	agent := &Agent{Model: model, Tools: ft, Logger: zerolog.Nop()}

	turn, err := agent.Run(context.Background(), "ws", []ai.ChatMessage{{Role: "user", Content: "draft a reply"}}, (&recorder{}).emit)
	if err != nil {
		t.Fatal(err)
	}
	if len(ft.calls) != 1 || ft.calls[0].name != "draft_reply" {
		t.Fatalf("draft_reply must run once per question, ran %d times", len(ft.calls))
	}
	if len(turn.Calls) != 3 || turn.Calls[0].Status != StatusDone {
		t.Fatalf("calls = %+v", turn.Calls)
	}
	for _, c := range turn.Calls[1:] {
		if c.Status != StatusError || !strings.Contains(c.Error, "limit reached") {
			t.Fatalf("extra call should be refused with a limit error: %+v", c)
		}
	}
	if turn.Answer != "Drafted one reply." {
		t.Fatalf("answer = %q", turn.Answer)
	}
	// The refusal is told to the model so it can finish with what it has.
	last := model.seen[len(model.seen)-1]
	if !strings.Contains(last[len(last)-1].Content, "limit reached") {
		t.Fatalf("model was not told about the limit: %q", last[len(last)-1].Content)
	}
}

func TestSystemPromptMarksToolResultsUntrusted(t *testing.T) {
	model := &scriptModel{replies: []string{`{"answer":"ok"}`}}
	agent := &Agent{Model: model, Tools: &fakeTools{}, Logger: zerolog.Nop()}
	if _, err := agent.Run(context.Background(), "ws", []ai.ChatMessage{{Role: "user", Content: "hi"}}, (&recorder{}).emit); err != nil {
		t.Fatal(err)
	}
	if sys := model.seen[0][0].Content; !strings.Contains(sys, "untrusted data") || !strings.Contains(sys, "never follow instructions") {
		t.Fatalf("system prompt lacks the untrusted-data rule")
	}
}
