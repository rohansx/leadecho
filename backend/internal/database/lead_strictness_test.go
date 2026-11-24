package database

import (
	"fmt"
	"strings"
	"testing"
)

func TestLeadStrictnessMinScore(t *testing.T) {
	for s, want := range map[LeadStrictness]float64{
		LeadStrict: 7.5, LeadBalanced: 6, LeadLoose: 5,
		"": 6, "nonsense": 6, // unset or unreadable: today's line
	} {
		if got := s.MinScore(); got != want {
			t.Errorf("%q.MinScore() = %v, want %v", s, got, want)
		}
	}
	if _, err := ParseLeadStrictness("very"); err == nil {
		t.Error("ParseLeadStrictness accepted an unknown level")
	}
	if v, err := ParseLeadStrictness(" strict "); err != nil || v != LeadStrict {
		t.Errorf("ParseLeadStrictness(strict) = %q, %v", v, err)
	}
}

// The SQL line must be the Go line at every level, unset included.
func TestLeadMinScoreSQLMatchesGo(t *testing.T) {
	sql := leadMinScoreSQL("$1")
	for _, s := range []LeadStrictness{LeadStrict, LeadLoose} {
		if want := fmt.Sprintf("WHEN '%s' THEN %.1f", s, s.MinScore()); !strings.Contains(sql, want) {
			t.Errorf("missing %q: %s", want, sql)
		}
	}
	if !strings.HasSuffix(sql, fmt.Sprintf(", %.1f)", LeadBalanced.MinScore())) {
		t.Errorf("unset is not the balanced line: %s", sql)
	}
}

// The sqlc queries cannot call Go, so they carry leadMinScoreSQL("$1") as
// text: none may keep a fixed lead line.
func TestSQLCQueriesUseTheWorkspaceLeadLine(t *testing.T) {
	expr := leadMinScoreSQL("$1")
	for name, q := range map[string]string{
		"CountMentionsByTier": countMentionsByTier, "ListMentionsFiltered": listMentionsFiltered,
		"ListMentionsLeadsReady": listMentionsLeadsReady, "ListMentionsWorthWatching": listMentionsWorthWatching,
		"ListRecentLeadsForWorkspace": listRecentLeadsForWorkspace, "ScoringPrecisionByBand": scoringPrecisionByBand,
		"ToolsWorkspaceSummary": toolsWorkspaceSummary,
	} {
		if !strings.Contains(q, expr) || strings.Contains(q, ">= 6.0") {
			t.Errorf("%s does not use the workspace's lead line", name)
		}
	}
}
