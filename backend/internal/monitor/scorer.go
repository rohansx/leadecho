package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/llm"
)

// batchScoreMentions runs the 4-stage auto-scoring pipeline on newly inserted mentions.
// Called after each workspace's crawl batch, before notifications.
func (m *Monitor) batchScoreMentions(ctx context.Context, wsID string, alerts []mentionAlert) {
	if len(alerts) == 0 {
		return
	}

	// Collect scoreable mentions (pass Stage 1 rules filter)
	var scoreable []mentionAlert
	for _, a := range alerts {
		if scoreStage1Rules(a.Content) {
			scoreable = append(scoreable, a)
		}
	}
	if len(scoreable) == 0 {
		return
	}

	// Stage 2: Batch embed + semantic matching (if embedder configured)
	type scored struct {
		alert      mentionAlert
		similarity float64
	}
	var candidates []scored

	if m.llmRouter != nil {
		// Batch embed all scoreable content
		texts := make([]string, len(scoreable))
		for i, a := range scoreable {
			text := a.Content
			if a.Title != "" {
				text = a.Title + "\n\n" + text
			}
			// Truncate to ~2000 chars to avoid huge embedding costs
			if len(text) > 2000 {
				text = text[:2000]
			}
			texts[i] = text
		}

		vectors, err := m.llmRouter.EmbedTexts(ctx, wsID, llm.TaskEmbedMentions, texts)
		if err != nil {
			m.logger.Error().Err(err).Msg("scorer: failed to batch embed mentions")
			// Fall through without embeddings — still try to classify
			for _, a := range scoreable {
				candidates = append(candidates, scored{alert: a, similarity: 0})
			}
		} else {
			// Store embeddings and find similar pain points
			for i, a := range scoreable {
				// The embedder returned fewer vectors than inputs (shouldn't
				// happen, but be defensive): don't silently drop the remaining
				// mentions — pass them to classification with no similarity,
				// matching the embed-failure fallback path above.
				if i >= len(vectors) {
					candidates = append(candidates, scored{alert: a, similarity: 0})
					continue
				}

				// Store the embedding
				if err := m.q.UpdateMentionEmbedding(ctx, database.UpdateMentionEmbeddingParams{
					ContentEmbedding: &vectors[i],
					ID:               a.ID,
				}); err != nil {
					m.logger.Error().Err(err).Str("mention_id", a.ID).Msg("scorer: failed to store embedding")
				}

				// Find similar pain points
				bestSim := 0.0
				similar, err := m.q.FindSimilarPainPoints(ctx, database.FindSimilarPainPointsParams{
					QueryEmbedding: &vectors[i],
					WorkspaceID:    wsID,
					Lim:            3,
				})
				if err != nil {
					m.logger.Error().Err(err).Msg("scorer: failed to find similar pain points")
				} else if len(similar) > 0 {
					bestSim = similar[0].Similarity
				}

				// Check if any profiles exist for this workspace
				profileCount, _ := m.q.CountMonitoringProfiles(ctx, wsID)

				// If profiles exist, only pass mentions with similarity > 0.40
				// If no profiles configured, pass everything (just classify)
				if profileCount > 0 && bestSim < 0.40 {
					// Low similarity, update metadata and skip
					m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
						ID:              a.ID,
						WorkspaceID:     wsID,
						ScoringMetadata: jsonBytes(map[string]any{"stage": "stage2_low_similarity", "best_similarity": bestSim, "auto_scored": true}),
						AwarenessLevel:  pgtype.Text{},
					})
					continue
				}

				candidates = append(candidates, scored{alert: a, similarity: bestSim})
			}
		}
	} else {
		// No embedder — pass all to classification
		for _, a := range scoreable {
			candidates = append(candidates, scored{alert: a, similarity: 0})
		}
	}

	if len(candidates) == 0 || m.llmRouter == nil {
		return
	}

	// Stage 3: Intent classification
	for _, c := range candidates {
		result, err := m.llmRouter.ClassifyIntent(ctx, wsID, c.alert.Title, c.alert.Content, c.alert.Platform)
		if err != nil {
			m.logger.Error().Err(err).Str("mention_id", c.alert.ID).Msg("scorer: classification failed")
			continue
		}

		meta := map[string]any{
			"stage":           "stage3_classified",
			"best_similarity": c.similarity,
			"auto_scored":     true,
			"reasoning":       result.Reasoning,
			"awareness_level": result.AwarenessLevel,
		}

		// Update mention with classification + awareness level
		m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
			ID:                    c.alert.ID,
			WorkspaceID:           c.alert.WorkspaceID,
			Intent:                database.NullIntentType{IntentType: database.IntentType(result.Intent), Valid: true},
			ConversionProbability: pgtype.Float4{Float32: float32(result.ConversionProbability), Valid: true},
			RelevanceScore:        pgtype.Float4{Float32: float32(result.RelevanceScore), Valid: true},
			ScoringMetadata:       jsonBytes(meta),
			AwarenessLevel:        pgtype.Text{String: result.AwarenessLevel, Valid: result.AwarenessLevel != ""},
		})

		m.publishMentionScored(ctx, c.alert, result)

		// Stage 4: Lead qualification (inline unless async qualifier consumer is enabled)
		if !m.qualifierAsync && result.RelevanceScore >= 7.0 {
			intent := database.IntentType(result.Intent)
			if intent == database.IntentTypeBuySignal ||
				intent == database.IntentTypeRecommendationAsk ||
				intent == database.IntentTypeComplaint {
				m.qualifyAsLead(ctx, c.alert, result)
			}
		}
	}

	m.logger.Info().
		Int("total", len(alerts)).
		Int("scored", len(candidates)).
		Str("workspace_id", wsID).
		Msg("scorer: batch scoring complete")
}

func (m *Monitor) publishMentionScored(ctx context.Context, alert mentionAlert, result *ai.ClassifyResult) {
	if !m.streamsEnabled || m.eventPublisher == nil {
		return
	}

	env, err := events.NewEnvelope(
		events.EventTypeMentionScored,
		events.AggregateTypeMention,
		alert.ID,
		"scorer",
		alert.WorkspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeMentionScored, alert.ID),
		events.MentionScoredPayload{
			MentionID:             alert.ID,
			WorkspaceID:           alert.WorkspaceID,
			Platform:              alert.Platform,
			Title:                 alert.Title,
			URL:                   alert.URL,
			Author:                alert.Author,
			Content:               alert.Content,
			Intent:                result.Intent,
			AwarenessLevel:        result.AwarenessLevel,
			RelevanceScore:        float32(result.RelevanceScore),
			ConversionProbability: float32(result.ConversionProbability),
			ScoringStage:          "stage3_classified",
		},
	)
	if err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: build mention.scored")
		return
	}
	if _, err := m.eventPublisher.Publish(ctx, env); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish mention.scored")
	}

	notifyEnv, err := events.NewEnvelope(
		events.EventTypeMentionNotificationRequest,
		events.AggregateTypeMention,
		alert.ID,
		"scorer",
		alert.WorkspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeMentionNotificationRequest, alert.ID),
		events.MentionNotificationRequestedPayload{
			MentionID:   alert.ID,
			WorkspaceID: alert.WorkspaceID,
			Platform:    alert.Platform,
			Keyword:     alert.Keyword,
			Title:       alert.Title,
			URL:         alert.URL,
			Author:      alert.Author,
			Score:       float32(result.RelevanceScore),
		},
	)
	if err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: build notification request")
		return
	}
	if _, err := m.eventPublisher.Publish(ctx, notifyEnv); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish notification request")
	}

	m.publishWorkflowTrigger(ctx, alert, result)
}

func (m *Monitor) publishWorkflowTrigger(ctx context.Context, alert mentionAlert, result *ai.ClassifyResult) {
	if !m.streamsEnabled || m.eventPublisher == nil {
		return
	}

	env, err := events.NewEnvelope(
		events.EventTypeWorkflowTriggerRequested,
		events.AggregateTypeWorkflow,
		alert.ID,
		"scorer",
		alert.WorkspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeWorkflowTriggerRequested, alert.ID),
		events.WorkflowTriggerRequestedPayload{
			MentionID:             alert.ID,
			WorkspaceID:           alert.WorkspaceID,
			Platform:              alert.Platform,
			Title:                 alert.Title,
			URL:                   alert.URL,
			Author:                alert.Author,
			Content:               alert.Content,
			Intent:                result.Intent,
			AwarenessLevel:        result.AwarenessLevel,
			RelevanceScore:        float32(result.RelevanceScore),
			ConversionProbability: float32(result.ConversionProbability),
		},
	)
	if err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: build workflow trigger")
		return
	}
	if _, err := m.eventPublisher.Publish(ctx, env); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish workflow trigger")
	}
}

// SignalAlert is the public-facing type for extension-sourced mentions entering the pipeline.
type SignalAlert struct {
	ID       string
	Platform string
	Title    string
	URL      string
	Author   string
	Content  string
}

// IngestSignals runs the extension mention pipeline on pre-inserted mentions.
// When streams are enabled it dual-writes/publishes mention.ingested events.
func (m *Monitor) IngestSignals(ctx context.Context, wsID string, alerts []SignalAlert) {
	m.ProcessIngestedSignals(ctx, wsID, alerts)
}

// scoreStage1Rules is a cheap rules-based filter. Returns true if the mention should proceed.
func scoreStage1Rules(content string) bool {
	if len(content) < 50 {
		return false
	}

	lower := strings.ToLower(content)

	// Skip obvious spam patterns
	spamPatterns := []string{
		"click here to win",
		"free money",
		"limited time offer",
		"act now",
		"buy followers",
		"crypto airdrop",
	}
	for _, p := range spamPatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	return true
}

// qualifyAsLead auto-creates a lead for high-intent mentions.
func (m *Monitor) qualifyAsLead(ctx context.Context, alert mentionAlert, result *ai.ClassifyResult) {
	_, err := m.q.CreateLead(ctx, database.CreateLeadParams{
		WorkspaceID: alert.WorkspaceID,
		MentionID:   pgUUID(alert.ID),
		Stage:       database.LeadStageProspect,
		Username:    pgtype.Text{String: alert.Author, Valid: alert.Author != ""},
		Platform:    database.NullPlatformType{PlatformType: database.PlatformType(alert.Platform), Valid: true},
		ProfileUrl:  pgtype.Text{},
		Tags:        []string{"auto-qualified"},
		Metadata:    jsonBytes(map[string]any{"auto_scored": true, "relevance": result.RelevanceScore, "intent": result.Intent}),
	})
	if err != nil {
		if !isDuplicateError(err) {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("scorer: failed to create lead")
		}
		return
	}
	m.logger.Info().Str("mention_id", alert.ID).Float64("relevance", result.RelevanceScore).Str("intent", result.Intent).Msg("scorer: auto-qualified lead")

	if !m.streamsEnabled || m.eventPublisher == nil {
		return
	}
	env, err := events.NewEnvelope(
		events.EventTypeMentionQualified,
		events.AggregateTypeMention,
		alert.ID,
		"lead_qualifier",
		alert.WorkspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeMentionQualified, alert.ID),
		events.MentionQualifiedPayload{
			MentionID:   alert.ID,
			WorkspaceID: alert.WorkspaceID,
			Intent:      result.Intent,
			Score:       float32(result.RelevanceScore),
		},
	)
	if err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: build mention.qualified")
		return
	}
	if _, err := m.eventPublisher.Publish(ctx, env); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish mention.qualified")
	}
}
