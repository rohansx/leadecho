package database

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
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

// The sqlc queries cannot call Go, so they carry leadScoreSQL as text: none
// may keep a fixed lead line or drop the company-signal exemption.
func TestSQLCQueriesUseTheWorkspaceLeadLine(t *testing.T) {
	for name, c := range map[string]struct{ q, col string }{
		"CountMentionsByTier": {countMentionsByTier, ""}, "ListMentionsFiltered": {listMentionsFiltered, ""},
		"ListMentionsLeadsReady": {listMentionsLeadsReady, ""}, "ListMentionsWorthWatching": {listMentionsWorthWatching, ""},
		"ListRecentLeadsForWorkspace": {listRecentLeadsForWorkspace, ""}, "ScoringPrecisionByBand": {scoringPrecisionByBand, ""},
		"ToolsWorkspaceSummary": {toolsWorkspaceSummary, "m."},
	} {
		if !strings.Contains(c.q, leadScoreSQL(c.col, "$1")) || strings.Contains(c.q, ">= 6.0") {
			t.Errorf("%s does not use the workspace's lead line", name)
		}
	}
}

// Strictness moves the line for posts only: a company signal is judged at
// the balanced line whatever the workspace chose.
func TestIsLeadExemptsCompanySignals(t *testing.T) {
	m := func(score float32, meta string) Mention {
		return Mention{RelevanceScore: pgtype.Float4{Float32: score, Valid: true},
			Intent: NullIntentType{IntentType: IntentTypeBuySignal, Valid: true}, PlatformMetadata: []byte(meta)}
	}
	const post, hiring, prospect = `{}`, `{"signal":"hiring"}`, `{"signal":"prospect"}`
	for _, c := range []struct {
		name string
		m    Mention
		line float64
		want bool
	}{
		{"confirmed company lead under strict", m(6.5, hiring), LeadStrict.MinScore(), true},
		{"prospect under strict", m(6.0, prospect), LeadStrict.MinScore(), true},
		{"unconfirmed hiring signal under loose", m(5.0, hiring), LeadLoose.MinScore(), false},
		{"post under strict", m(7.0, post), LeadStrict.MinScore(), false},
		{"post under loose", m(5.0, post), LeadLoose.MinScore(), true},
		{"no metadata is a post", m(5.0, ""), LeadLoose.MinScore(), true},
	} {
		if got := c.m.IsLead(c.line); got != c.want {
			t.Errorf("%s: IsLead = %v, want %v", c.name, got, c.want)
		}
	}
}
