package database

import (
	"strings"
	"testing"
)

func TestListMentionsComposedParams_buildWhere_queueAutoFlowing(t *testing.T) {
	where, args := (ListMentionsComposedParams{
		WorkspaceID: "ws-1",
		Queue:       "auto_flowing",
	}).buildWhere()

	if len(args) != 1 || args[0] != "ws-1" {
		t.Fatalf("args = %v, want workspace id only", args)
	}
	for _, want := range []string{"relevance_score >= 7.0", "replies r", "needs_escalation"} {
		if !strings.Contains(where, want) {
			t.Fatalf("where missing %q: %s", want, where)
		}
	}
}

func TestListMentionsComposedParams_buildWhere_queueEscalations(t *testing.T) {
	where, _ := (ListMentionsComposedParams{
		WorkspaceID: "ws-1",
		Queue:       "escalations",
	}).buildWhere()

	for _, want := range []string{"needs_escalation", "status = 'new'", "NOT EXISTS"} {
		if !strings.Contains(where, want) {
			t.Fatalf("where missing %q: %s", want, where)
		}
	}
}

func TestListMentionsComposedParams_buildWhere_escalationNeedsDraft(t *testing.T) {
	where, _ := (ListMentionsComposedParams{
		WorkspaceID:    "ws-1",
		Queue:          "escalations",
		EscalationKind: "needs_draft",
	}).buildWhere()

	if strings.Contains(where, "needs_escalation") {
		t.Fatalf("needs_draft slice should not require escalation flag: %s", where)
	}
	if !strings.Contains(where, "NOT EXISTS") {
		t.Fatalf("needs_draft should require no reply: %s", where)
	}
}

func TestListMentionsComposedParams_buildWhere_queueAll(t *testing.T) {
	where, _ := (ListMentionsComposedParams{
		WorkspaceID: "ws-1",
		Queue:       "all",
	}).buildWhere()

	if !strings.Contains(where, "status NOT IN ('spam', 'archived')") {
		t.Fatalf("all queue should exclude spam/archived: %s", where)
	}
	if strings.Contains(where, "replies r") {
		t.Fatalf("all queue should not require replies: %s", where)
	}
}

func TestListMentionsComposedParams_buildWhere_actionRequired(t *testing.T) {
	where, _ := (ListMentionsComposedParams{
		WorkspaceID: "ws-1",
		Queue:       "action_required",
	}).buildWhere()

	if !strings.Contains(where, "replies r") {
		t.Fatalf("action_required should include reply predicates: %s", where)
	}
}
