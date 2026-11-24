package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Lead strictness: how sure a post must be to count as a lead, under
// workspaces.settings.lead_strictness. Unset is "balanced", the line every
// workspace had before the setting existed. Company signals (hiring, funding,
// prospects: platform_metadata.signal set) are exempt and always use the
// balanced line: their score is a fit score from a paid, confirmed lookup, not
// the classifier's confidence. Hand-written (no sqlc codegen).

type LeadStrictness string

const (
	LeadStrictnessKey                 = "lead_strictness"
	LeadStrict         LeadStrictness = "strict"   // fewer, surer leads
	LeadBalanced       LeadStrictness = "balanced" // LeadMinScore
	LeadLoose          LeadStrictness = "loose"    // more leads
	strictLeadMinScore                = 7.5
	looseLeadMinScore                 = 5.0
)

// MinScore is the lowest score that is a lead at this strictness; anything
// unknown is balanced.
func (s LeadStrictness) MinScore() float64 {
	switch s {
	case LeadStrict:
		return strictLeadMinScore
	case LeadLoose:
		return looseLeadMinScore
	}
	return LeadMinScore
}

// ParseLeadStrictness accepts the three levels only.
func ParseLeadStrictness(s string) (LeadStrictness, error) {
	switch v := LeadStrictness(strings.TrimSpace(s)); v {
	case LeadStrict, LeadBalanced, LeadLoose:
		return v, nil
	}
	return "", fmt.Errorf("strictness must be %s, %s or %s", LeadStrict, LeadBalanced, LeadLoose)
}

// leadMinScoreSQL is MinScore as SQL for the workspace whose id is wsExpr (a
// column or a parameter), so every query draws the lead line where Go does.
func leadMinScoreSQL(wsExpr string) string {
	return fmt.Sprintf(`COALESCE((SELECT CASE w.settings->>'%s' WHEN '%s' THEN %.1f WHEN '%s' THEN %.1f END FROM workspaces w WHERE w.id = %s), %.1f)`,
		LeadStrictnessKey, LeadStrict, strictLeadMinScore, LeadLoose, looseLeadMinScore, wsExpr, LeadMinScore)
}

// LeadLine is a row's lead line: postLine (the workspace's) for a post, the
// balanced line for a company signal.
func LeadLine(postLine float64, companySignal bool) float64 {
	if companySignal {
		return LeadMinScore
	}
	return postLine
}

// leadScoreSQL is "the row's score clears its lead line" as SQL (LeadLine):
// col prefixes the row's columns ("" or "m."), wsExpr is the workspace id.
// The LEAST(...) bound keeps the (workspace_id, relevance_score) index usable.
func leadScoreSQL(col, wsExpr string) string {
	return fmt.Sprintf(`%[1]srelevance_score >= LEAST(%.1[2]f, %[3]s) AND %[1]srelevance_score >= CASE WHEN COALESCE(%[1]splatform_metadata->>'signal', '') <> '' THEN %.1[2]f ELSE %[3]s END`,
		col, LeadMinScore, leadMinScoreSQL(wsExpr))
}

// GetLeadStrictness returns the workspace's level; balanced when none is
// stored, and balanced with the error when it cannot be read.
func (q *Queries) GetLeadStrictness(ctx context.Context, workspaceID string) (LeadStrictness, error) {
	raw, err := q.GetWorkspaceSettingsKey(ctx, workspaceID, LeadStrictnessKey)
	if err != nil {
		return LeadBalanced, err
	}
	var s string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s) // not a string: unset
	}
	if v, err := ParseLeadStrictness(s); err == nil {
		return v, nil
	}
	return LeadBalanced, nil
}

// LeadMinScoreFor is the workspace's lead line for posts (balanced, with the
// error, when it cannot be read).
func (q *Queries) LeadMinScoreFor(ctx context.Context, workspaceID string) (float64, error) {
	s, err := q.GetLeadStrictness(ctx, workspaceID)
	return s.MinScore(), err
}

// SetLeadStrictness stores the workspace's level.
func (q *Queries) SetLeadStrictness(ctx context.Context, workspaceID string, s LeadStrictness) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return q.SetWorkspaceSettingsKey(ctx, workspaceID, LeadStrictnessKey, raw)
}
