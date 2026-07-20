package llm

import (
	"context"

	pgvector "github.com/pgvector/pgvector-go"

	"leadecho/internal/ai"
)

// MentionScorer covers LLM calls used by the mention scoring pipeline.
type MentionScorer interface {
	FilterMention(ctx context.Context, workspaceID, title, content, platform string) (*ai.FilterResult, error)
	EmbedTexts(ctx context.Context, workspaceID string, task Task, texts []string) ([]pgvector.Vector, error)
	ClassifyIntent(ctx context.Context, workspaceID, title, content, platform string) (*ai.ClassifyResult, error)
}

// ReplyGenerator covers LLM calls used by reply drafting.
type ReplyGenerator interface {
	PreFilterForReply(ctx context.Context, workspaceID, title, content, platform, intent string) (*ai.PreFilterResult, error)
	DraftReplyEnhanced(ctx context.Context, workspaceID string, opts ai.DraftReplyOptions) (*ai.EnhancedDraftResult, error)
}
