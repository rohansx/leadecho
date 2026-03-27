package monitor

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
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

	if m.embedder != nil {
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

		vectors, err := m.embedder.EmbedTexts(ctx, texts)
		if err != nil {
			m.logger.Error().Err(err).Msg("scorer: failed to batch embed mentions")
			// Fall through without embeddings — still try to classify
			for _, a := range scoreable {
				candidates = append(candidates, scored{alert: a, similarity: 0})
			}
		} else {
			// Store embeddings and find similar pain points
			for i, a := range scoreable {
				if i >= len(vectors) {
					break
				}

				// Store the embedding
				if err := m.q.UpdateMentionEmbedding(ctx, database.UpdateMentionEmbeddingParams{
					ContentEmbedding: vectors[i],
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

	if len(candidates) == 0 || m.aiProvider == nil {
		return
	}

	// Stage 3: Intent classification
	for _, c := range candidates {
		result, err := ai.ClassifyIntent(ctx, *m.aiProvider, c.alert.Title, c.alert.Content, c.alert.Platform)
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

		// Stage 4: Lead qualification
		if result.RelevanceScore >= 7.0 {
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

// SignalAlert is the public-facing type for extension-sourced mentions entering the pipeline.
type SignalAlert struct {
	ID       string
	Platform string
	Title    string
	URL      string
	Author   string
	Content  string
}

// IngestSignals runs the 4-stage scoring + notification pipeline on pre-inserted mentions
// from the Chrome extension. Alerts must already be persisted to the DB with valid IDs.
func (m *Monitor) IngestSignals(ctx context.Context, wsID string, alerts []SignalAlert) {
	ma := make([]mentionAlert, len(alerts))
	for i, a := range alerts {
		ma[i] = mentionAlert{
			ID:          a.ID,
			WorkspaceID: wsID,
			Platform:    a.Platform,
			Title:       a.Title,
			URL:         a.URL,
			Author:      a.Author,
			Content:     a.Content,
		}
	}
	m.batchScoreMentions(ctx, wsID, ma)
	m.notifyNewMentions(ctx, wsID, ma)
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
}
