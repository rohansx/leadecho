package llm

import (
	"context"
	"strings"

	"leadecho/internal/ai"
)

// TaskChat routes the in-app assistant ("Ask LeadEcho", P2 #15); cheap tier by default.
const TaskChat Task = "chat"

// Chat runs one assistant step; tokens land in llm_usage_events (task=chat).
func (r *Router) Chat(ctx context.Context, workspaceID string, msgs []ai.ChatMessage) (string, error) {
	var out string
	err := r.withChatProviderUsage(ctx, workspaceID, TaskChat, func(p ai.Provider) (ai.Usage, error) {
		res, u, err := ai.Chat(ctx, p, msgs)
		out = res
		return u, err
	})
	return out, err
}

// ChatAvailable reports whether the workspace has a usable chat provider key.
func (r *Router) ChatAvailable(ctx context.Context, workspaceID string) bool {
	cfg, legacy, err := r.loadConfig(ctx, workspaceID)
	if err != nil {
		return false
	}
	for _, t := range r.targetsForTask(TaskChat, cfg, legacy) {
		if _, _, err := r.resolveKey(strings.ToLower(t.Provider), cfg, legacy); err == nil {
			return true
		}
	}
	return false
}
