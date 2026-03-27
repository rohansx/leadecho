package handler

import (
	"math/rand/v2"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

type AIHandler struct {
	q            *database.Queries
	glmAPIKey    string
	openAIAPIKey string
	scrapling    *browser.ScraplingClient
}

func NewAIHandler(q *database.Queries, glmAPIKey, openAIAPIKey string, scrapling *browser.ScraplingClient) *AIHandler {
	return &AIHandler{q: q, glmAPIKey: glmAPIKey, openAIAPIKey: openAIAPIKey, scrapling: scrapling}
}

// getProvider returns the system LLM provider. Tries GLM first, then OpenAI.
func (h *AIHandler) getProvider() *ai.Provider {
	if h.glmAPIKey != "" {
		p := ai.DefaultProvider("glm", h.glmAPIKey)
		return &p
	}
	if h.openAIAPIKey != "" {
		p := ai.DefaultProvider("openai", h.openAIAPIKey)
		return &p
	}
	return nil
}

// Classify classifies a mention's intent using the configured LLM.
// POST /mentions/{id}/classify
func (h *AIHandler) Classify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	id := chi.URLParam(r, "id")

	// Get the mention
	mention, err := h.q.GetMention(ctx, database.GetMentionParams{
		ID:          id,
		WorkspaceID: wsID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "mention not found")
		return
	}

	// Get LLM provider
	provider := h.getProvider()
	if provider == nil {
		writeError(w, http.StatusBadRequest, "no AI provider configured — set GLM_API_KEY or OPENAI_API_KEY in .env")
		return
	}

	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}

	// Classify
	result, err := ai.ClassifyIntent(ctx, *provider, title, mention.Content, string(mention.Platform))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "classification failed: "+err.Error())
		return
	}

	// Update mention with classification
	updated, err := h.q.UpdateMentionIntent(ctx, database.UpdateMentionIntentParams{
		ID:                    id,
		WorkspaceID:           wsID,
		Intent:                database.NullIntentType{IntentType: database.IntentType(result.Intent), Valid: true},
		ConversionProbability: pgtype.Float4{Float32: float32(result.ConversionProbability), Valid: true},
		RelevanceScore:        pgtype.Float4{Float32: float32(result.RelevanceScore), Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save classification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mention":   mentionToResponse(updated),
		"reasoning": result.Reasoning,
	})
}

// DraftReply generates an AI reply draft for a mention using the two-stage pipeline.
// Stage 1: Pre-filter (is it worth replying?)
// Stage 2: Enhanced draft with thread context and template rotation.
// POST /mentions/{id}/draft-reply
func (h *AIHandler) DraftReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	id := chi.URLParam(r, "id")

	// Get the mention
	mention, err := h.q.GetMention(ctx, database.GetMentionParams{
		ID:          id,
		WorkspaceID: wsID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "mention not found")
		return
	}

	// Get LLM provider
	provider := h.getProvider()
	if provider == nil {
		writeError(w, http.StatusBadRequest, "no AI provider configured — set GLM_API_KEY or OPENAI_API_KEY in .env")
		return
	}

	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}
	intent := "general"
	if mention.Intent.Valid {
		intent = string(mention.Intent.IntentType)
	}

	// Stage 1: Pre-filter — is this mention worth replying to?
	preFilter, err := ai.PreFilterForReply(ctx, *provider, title, mention.Content, string(mention.Platform), intent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pre-filter failed: "+err.Error())
		return
	}

	// Update awareness level from pre-filter
	if preFilter.AwarenessLevel != "" {
		h.q.UpdateMentionAwarenessLevel(ctx, database.UpdateMentionAwarenessLevelParams{
			AwarenessLevel: pgtype.Text{String: preFilter.AwarenessLevel, Valid: true},
			ID:             id,
			WorkspaceID:    wsID,
		})
	}

	if !preFilter.ShouldReply {
		writeJSON(w, http.StatusOK, map[string]any{
			"should_reply":    false,
			"reason":          preFilter.Reason,
			"awareness_level": preFilter.AwarenessLevel,
		})
		return
	}

	// Stage 2: Fetch thread context
	threadCtx, _ := monitor.FetchThreadContext(ctx, h.q, h.scrapling, mention)

	// Gather KB context from documents
	kbContext := ""
	docs, err := h.q.ListDocuments(ctx, wsID)
	if err == nil && len(docs) > 0 {
		var parts []string
		for _, d := range docs {
			if len(parts) >= 3 {
				break
			}
			snippet := d.Content
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			parts = append(parts, d.Title+": "+snippet)
		}
		kbContext = joinStrings(parts, "\n---\n")
	}

	// Select template style based on intent + awareness level
	templateStyle := selectTemplateStyle(intent, preFilter.AwarenessLevel)

	// Stage 3: Enhanced draft with full context
	result, err := ai.DraftReplyEnhanced(ctx, *provider, ai.DraftReplyOptions{
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
		writeError(w, http.StatusInternalServerError, "draft generation failed: "+err.Error())
		return
	}

	// Save as draft reply in DB
	reply, err := h.q.CreateReply(ctx, database.CreateReplyParams{
		MentionID:         id,
		WorkspaceID:       wsID,
		Content:           result.Reply,
		Status:            database.ReplyStatusDraft,
		TemplateStyle:     pgtype.Text{String: result.TemplateStyle, Valid: true},
		ThreadContextUsed: threadCtx != "",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save reply draft")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reply":              replyToResponse(reply),
		"tone":               result.Tone,
		"template_style":     result.TemplateStyle,
		"should_reply":       true,
		"awareness_level":    preFilter.AwarenessLevel,
		"thread_context_used": threadCtx != "",
	})
}

// selectTemplateStyle picks a reply template based on intent and awareness level.
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

	// Override for low-awareness: always be helpful first, no product push
	if awareness == "problem_aware" {
		styles = []string{"casual_helpful", "storytelling", "casual_helpful"}
	}

	return styles[rand.IntN(len(styles))]
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}
