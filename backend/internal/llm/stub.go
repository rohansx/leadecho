package llm

import (
	"context"

	pgvector "github.com/pgvector/pgvector-go"

	"leadecho/internal/ai"
)

// Stub provides deterministic LLM responses for integration and unit tests.
type Stub struct {
	ClassifyResult  *ai.ClassifyResult
	PreFilterResult *ai.PreFilterResult
	DraftResult     *ai.EnhancedDraftResult
	EmbedDimensions int
}

func (s *Stub) ClassifyIntent(ctx context.Context, workspaceID, title, content, platform string) (*ai.ClassifyResult, error) {
	if s.ClassifyResult != nil {
		return s.ClassifyResult, nil
	}
	return &ai.ClassifyResult{
		Intent:                "buy_signal",
		ConversionProbability: 0.85,
		RelevanceScore:        9.0,
		Reasoning:             "stub: high purchase intent",
		AwarenessLevel:        "solution_aware",
	}, nil
}

func (s *Stub) EmbedTexts(ctx context.Context, workspaceID string, task Task, texts []string) ([]pgvector.Vector, error) {
	dim := s.EmbedDimensions
	if dim <= 0 {
		dim = 1024
	}
	out := make([]pgvector.Vector, len(texts))
	for i := range texts {
		vec := make([]float32, dim)
		vec[0] = 0.25
		out[i] = pgvector.NewVector(vec)
	}
	return out, nil
}

func (s *Stub) PreFilterForReply(ctx context.Context, workspaceID, title, content, platform, intent string) (*ai.PreFilterResult, error) {
	if s.PreFilterResult != nil {
		return s.PreFilterResult, nil
	}
	return &ai.PreFilterResult{
		ShouldReply:    true,
		Reason:         "stub: worth replying",
		AwarenessLevel: "solution_aware",
	}, nil
}

func (s *Stub) DraftReplyEnhanced(ctx context.Context, workspaceID string, opts ai.DraftReplyOptions) (*ai.EnhancedDraftResult, error) {
	if s.DraftResult != nil {
		return s.DraftResult, nil
	}
	return &ai.EnhancedDraftResult{
		Reply:         "Happy to share what worked for our team — happy to DM details if useful.",
		Tone:          "helpful",
		TemplateStyle: "value_first",
	}, nil
}
