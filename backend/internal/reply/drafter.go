package reply

import (
	"context"
	"encoding/json"
	"math/rand/v2"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/llm"
	"leadecho/internal/monitor"
)

func (d *Drafter) markEscalation(ctx context.Context, wsID, mentionID string, existingMeta []byte, reason string) {
	meta := map[string]any{}
	if len(existingMeta) > 0 {
		_ = json.Unmarshal(existingMeta, &meta)
	}
	meta["needs_escalation"] = true
	meta["escalation_reason"] = reason
	b, _ := json.Marshal(meta)
	_, _ = d.q.PatchMentionScoringMetadata(ctx, database.PatchMentionScoringMetadataParams{
		ID:              mentionID,
		WorkspaceID:     wsID,
		ScoringMetadata: b,
	})
}

// KBRetriever supplies semantic knowledge-base context for reply drafting.
type KBRetriever interface {
	Retrieve(ctx context.Context, workspaceID, query string, topK int32) (string, error)
}

type Drafter struct {
	q         *database.Queries
	llmRouter llm.ReplyGenerator
	scrapling *browser.ScraplingClient
	kb        KBRetriever
}

func NewDrafter(q *database.Queries, llmRouter llm.ReplyGenerator, scrapling *browser.ScraplingClient, kb KBRetriever) *Drafter {
	return &Drafter{q: q, llmRouter: llmRouter, scrapling: scrapling, kb: kb}
}

type DraftResult struct {
	Reply             database.Reply
	Tone              string
	TemplateStyle     string
	ShouldReply       bool
	Reason            string
	AwarenessLevel    string
	ThreadContextUsed bool
}

func (d *Drafter) DraftForMention(ctx context.Context, wsID, mentionID string) (*DraftResult, error) {
	mention, err := d.q.GetMention(ctx, database.GetMentionParams{
		ID:          mentionID,
		WorkspaceID: wsID,
	})
	if err != nil {
		return nil, err
	}

	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}
	intent := "general"
	if mention.Intent.Valid {
		intent = string(mention.Intent.IntentType)
	}

	preFilter, err := d.llmRouter.PreFilterForReply(ctx, wsID, title, mention.Content, string(mention.Platform), intent)
	if err != nil {
		return nil, err
	}

	if preFilter.AwarenessLevel != "" {
		_ = d.q.UpdateMentionAwarenessLevel(ctx, database.UpdateMentionAwarenessLevelParams{
			AwarenessLevel: pgtype.Text{String: preFilter.AwarenessLevel, Valid: true},
			ID:             mentionID,
			WorkspaceID:    wsID,
		})
	}

	if !preFilter.ShouldReply {
		d.markEscalation(ctx, wsID, mentionID, mention.ScoringMetadata, preFilter.Reason)
		return &DraftResult{
			ShouldReply:    false,
			Reason:         preFilter.Reason,
			AwarenessLevel: preFilter.AwarenessLevel,
		}, nil
	}

	threadCtx, _ := monitor.FetchThreadContext(ctx, d.q, d.scrapling, mention)

	kbContext := ""
	if d.kb != nil {
		query := mention.Content
		if title != "" {
			query = title + "\n\n" + query
		}
		if ctxText, err := d.kb.Retrieve(ctx, wsID, query, 5); err == nil {
			kbContext = ctxText
		}
	}

	templateStyle := selectTemplateStyle(intent, preFilter.AwarenessLevel)

	result, err := d.llmRouter.DraftReplyEnhanced(ctx, wsID, ai.DraftReplyOptions{
		Title:          title,
		Content:        mention.Content,
		Platform:       string(mention.Platform),
		Intent:         intent,
		AwarenessLevel: preFilter.AwarenessLevel,
		ThreadContext:  threadCtx,
		KBContext:      kbContext,
		TemplateStyle:  templateStyle,
	})
	if err != nil {
		return nil, err
	}

	reply, err := d.q.CreateReply(ctx, database.CreateReplyParams{
		MentionID:         mentionID,
		WorkspaceID:       wsID,
		Content:           result.Reply,
		Status:            database.ReplyStatusDraft,
		TemplateStyle:     pgtype.Text{String: result.TemplateStyle, Valid: true},
		ThreadContextUsed: threadCtx != "",
	})
	if err != nil {
		return nil, err
	}

	return &DraftResult{
		Reply:             reply,
		Tone:              result.Tone,
		TemplateStyle:     result.TemplateStyle,
		ShouldReply:       true,
		AwarenessLevel:    preFilter.AwarenessLevel,
		ThreadContextUsed: threadCtx != "",
	}, nil
}

func selectTemplateStyle(intent, awareness string) string {
	styles := []string{"value_first"}

	switch intent {
	case "recommendation_ask":
		styles = []string{"value_first", "value_first", "storytelling"}
	case "complaint":
		styles = []string{"storytelling", "storytelling", "value_first"}
	case "comparison":
		styles = []string{"technical_deep_dive", "contrarian", "value_first"}
	case "general":
		styles = []string{"casual_helpful", "casual_helpful", "value_first"}
	case "buy_signal":
		styles = []string{"value_first", "technical_deep_dive", "value_first"}
	}

	if awareness == "problem_aware" {
		styles = []string{"casual_helpful", "storytelling", "casual_helpful"}
	}

	return styles[rand.IntN(len(styles))]
}
