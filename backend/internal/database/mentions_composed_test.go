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
	for _, want := range []string{"relevance_score >= 6.0", "comparison", "replies r", "needs_escalation"} {
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

func TestListMentionsComposedParams_buildWhere_queueAllExplicitArchived(t *testing.T) {
	for _, st := range []string{"archived", "spam"} {
		where, args := (ListMentionsComposedParams{WorkspaceID: "ws-1", Queue: "all", Status: st}).buildWhere()
		if strings.Contains(where, "NOT IN ('spam', 'archived')") {
			t.Fatalf("status=%s must not be excluded by the default filter: %s", st, where)
		}
		found := false
		for _, a := range args {
			if a == st {
				found = true
			}
		}
		if !found {
			t.Fatalf("status=%s should be bound as a filter arg: %v", st, args)
		}
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

// scoring: tag + sentiment filters
func TestListMentionsComposedParams_buildWhere_tagAndSentiment(t *testing.T) {
	where, args := (ListMentionsComposedParams{
		WorkspaceID: "ws-1",
		Tag:         "hiring",
		Sentiment:   "negative",
	}).buildWhere()
	for _, want := range []string{"$2 = ANY(intent_tags)", "sentiment = $3"} {
		if !strings.Contains(where, want) {
			t.Fatalf("where missing %q: %s", want, where)
		}
	}
	if len(args) != 3 || args[1] != "hiring" || args[2] != "negative" {
		t.Fatalf("args = %v", args)
	}
}

// Search looks in the post, its title and its author with one parameter, so
// a company or a person can be found by name.
func TestListMentionsComposedParams_buildWhere_searchCoversTitleAndAuthor(t *testing.T) {
	where, args := (ListMentionsComposedParams{WorkspaceID: "ws-1", Queue: "all", Search: "toku"}).buildWhere()
	if len(args) != 2 || args[1] != "toku" {
		t.Fatalf("args = %v, want the workspace id and the search text once", args)
	}
	for _, want := range []string{"plainto_tsquery('english', $2)", "title ILIKE '%' || $2 || '%'", "author_username ILIKE '%' || $2 || '%'"} {
		if !strings.Contains(where, want) {
			t.Fatalf("where missing %q: %s", want, where)
		}
	}
}
