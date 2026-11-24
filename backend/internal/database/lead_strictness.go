package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Lead strictness: how sure a post must be to count as a lead, under
// workspaces.settings.lead_strictness. Unset is "balanced", the line every
// workspace had before the setting existed. Hand-written (no sqlc codegen).

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

// GetLeadStrictness returns the workspace's level; balanced when none is
// stored or it cannot be read.
func (q *Queries) GetLeadStrictness(ctx context.Context, workspaceID string) LeadStrictness {
	var s string
	if raw, err := q.GetWorkspaceSettingsKey(ctx, workspaceID, LeadStrictnessKey); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	if v, err := ParseLeadStrictness(s); err == nil {
		return v
	}
	return LeadBalanced
}

// LeadMinScoreFor is the workspace's lead line.
func (q *Queries) LeadMinScoreFor(ctx context.Context, workspaceID string) float64 {
	return q.GetLeadStrictness(ctx, workspaceID).MinScore()
}

// SetLeadStrictness stores the workspace's level.
func (q *Queries) SetLeadStrictness(ctx context.Context, workspaceID string, s LeadStrictness) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return q.SetWorkspaceSettingsKey(ctx, workspaceID, LeadStrictnessKey, raw)
}
