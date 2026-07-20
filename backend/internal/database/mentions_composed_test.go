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
