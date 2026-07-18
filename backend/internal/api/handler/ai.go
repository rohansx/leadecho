package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
	"leadecho/internal/llm"
	"leadecho/internal/reply"
)

type AIHandler struct {
	q         *database.Queries
	llmRouter *llm.Router
	scrapling *browser.ScraplingClient
	drafter   *reply.Drafter
	publisher         *publishers.Publisher
	replyDrafterAsync bool
}

func NewAIHandler(q *database.Queries, llmRouter *llm.Router, scrapling *browser.ScraplingClient, drafter *reply.Drafter, publisher *publishers.Publisher, replyDrafterAsync bool) *AIHandler {
	return &AIHandler{q: q, llmRouter: llmRouter, scrapling: scrapling, drafter: drafter, publisher: publisher, replyDrafterAsync: replyDrafterAsync}
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

	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}

	// Classify
	result, err := h.llmRouter.ClassifyIntent(ctx, wsID, title, mention.Content, string(mention.Platform))
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
// When streams + reply drafter consumer are enabled, publishes reply.draft_requested and returns 202.
// POST /mentions/{id}/draft-reply
func (h *AIHandler) DraftReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	id := chi.URLParam(r, "id")

	if h.replyDrafterAsync && h.publisher != nil {
		env, err := events.NewEnvelope(
			events.EventTypeReplyDraftRequested,
			events.AggregateTypeReply,
			id,
			"api",
			wsID,
			events.EventTypeReplyDraftRequested+":"+id,
			events.ReplyDraftRequestedPayload{
				MentionID:   id,
				WorkspaceID: wsID,
				Source:      "api",
			},
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build draft event")
			return
		}
		if _, err := h.publisher.Publish(ctx, env); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enqueue draft request")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":     "queued",
			"mention_id": id,
			"event_id":   env.EventID,
		})
		return
	}

	result, err := h.drafter.DraftForMention(ctx, wsID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "draft generation failed: "+err.Error())
		return
	}
	if !result.ShouldReply {
		writeJSON(w, http.StatusOK, map[string]any{
			"should_reply":    false,
			"reason":          result.Reason,
			"awareness_level": result.AwarenessLevel,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reply":               replyToResponse(result.Reply),
		"tone":                result.Tone,
		"template_style":      result.TemplateStyle,
		"should_reply":        true,
		"awareness_level":     result.AwarenessLevel,
		"thread_context_used": result.ThreadContextUsed,
	})
}
