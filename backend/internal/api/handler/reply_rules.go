package handler

import (
	"context"

	"leadecho/internal/database"
	"leadecho/internal/reply"
	"leadecho/internal/subrules"
)

// SetSubredditRules attaches subreddit rules and warnings to Reddit replies.
func (h *ReplyHandler) SetSubredditRules(s *subrules.Service) { h.rules = s }

// withRules adds the subreddit's rules summary and the reply's warnings
// against it. Best effort: a lookup failure leaves the reply as it was.
func (h *ReplyHandler) withRules(ctx context.Context, resp ReplyResponse) ReplyResponse {
	if h.rules == nil {
		return resp
	}
	m, err := h.q.GetMention(ctx, database.GetMentionParams{ID: resp.MentionID, WorkspaceID: resp.WorkspaceID})
	if err != nil {
		return resp
	}
	sum := reply.RulesFor(ctx, h.rules, m)
	if sum == nil {
		return resp
	}
	text := resp.Content
	if resp.EditedContent != nil && *resp.EditedContent != "" {
		text = *resp.EditedContent
	}
	resp.CommunityRules = sum
	resp.RuleWarnings = subrules.Check(sum, text, h.brandNames(ctx, resp.WorkspaceID))
	return resp
}

// brandNames are the workspace's product names (its monitoring profiles).
func (h *ReplyHandler) brandNames(ctx context.Context, wsID string) []string {
	profiles, err := h.q.ListMonitoringProfiles(ctx, wsID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}
