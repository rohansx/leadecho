package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type AIHandler struct {
	q            *database.Queries
	glmAPIKey    string
	openAIAPIKey string
}

func NewAIHandler(q *database.Queries, glmAPIKey, openAIAPIKey string) *AIHandler {
	return &AIHandler{q: q, glmAPIKey: glmAPIKey, openAIAPIKey: openAIAPIKey}
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

// DraftReply generates an AI reply draft for a mention.
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

	// Draft reply
	result, err := ai.DraftReply(ctx, *provider, title, mention.Content, string(mention.Platform), intent, kbContext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "draft generation failed: "+err.Error())
		return
	}

	// Save as draft reply in DB
	reply, err := h.q.CreateReply(ctx, database.CreateReplyParams{
		MentionID:   id,
		WorkspaceID: wsID,
		Content:     result.Reply,
		Status:      database.ReplyStatusDraft,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save reply draft")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reply": replyToResponse(reply),
		"tone":  result.Tone,
	})
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
