package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
	"leadecho/internal/llm"
	"leadecho/internal/workflow"
)

// batchScoreMentions runs the 4-stage auto-scoring pipeline on newly inserted mentions.
// Called after each workspace's crawl batch, before notifications.
func (m *Monitor) batchScoreMentions(ctx context.Context, wsID string, alerts []mentionAlert) {
	if len(alerts) == 0 {
		return
	}

	// Collect scoreable mentions (pass Stage 0 rules filter)
	var scoreable []mentionAlert
	for _, a := range alerts {
		if scoreStage1Rules(a.Content) {
			scoreable = append(scoreable, a)
		} else {
			m.markStageFiltered(ctx, a, "stage0_rules", "failed rules pre-filter")
		}
	}
	if len(scoreable) == 0 {
		return
	}

	// billing: one credit per post sent to the AI. Posts beyond the balance
	// stay unscored (no stage) and the backfill scores them once credits return.
	if m.llmRouter != nil && m.credits.Enforced() {
		taken, err := m.credits.Take(ctx, wsID, len(scoreable), billing.ReasonPost)
		if err != nil {
			m.logger.Error().Err(err).Str("workspace_id", wsID).Msg("scorer: credit check failed, not scoring")
			return
		}
		if taken < len(scoreable) {
			m.logger.Info().Int("left_unscored", len(scoreable)-taken).Str("workspace_id", wsID).Msg("scorer: out of credits")
			scoreable = scoreable[:taken]
		}
		if len(scoreable) == 0 {
			return
		}
	}

	// Stage 1: LLM spam/noise filter (cheap model via TaskFilter)
	if m.llmRouter != nil {
		keep := make([]bool, len(scoreable))
		m.forEachConcurrent(ctx, len(scoreable), func(i int) {
			a := scoreable[i]
			fr, err := m.llmRouter.FilterMention(ctx, wsID, a.Title, a.Content, a.Platform)
			if err != nil {
				m.logger.Warn().Err(err).Str("mention_id", a.ID).Msg("scorer: stage1 filter failed, fail-open")
				keep[i] = true
				return
			}
			if !fr.Pass {
				m.markStageFiltered(ctx, a, "stage1_spam_filter", fr.Reason)
				return
			}
			keep[i] = true
		})
		var passed []mentionAlert
		for i, a := range scoreable {
			if keep[i] {
				passed = append(passed, a)
			}
		}
		scoreable = passed
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
		m.ensurePainPointEmbeddings(ctx, wsID)
		// Batch embed all scoreable content
		texts := make([]string, len(scoreable))
		for i, a := range scoreable {
			text := a.Content
			if a.Title != "" {
				text = a.Title + "\n\n" + text
			}
			// Truncate to ~2000 chars to avoid huge embedding costs.
			// Slice on a rune boundary: a plain text[:2000] byte slice can cut a
			// multi-byte character in half and hand invalid UTF-8 to the embedding
			// provider, which matters for the non-English communities we crawl.
			text = truncateRunes(text, 2000)
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

				if lowPainPointSimilarity(len(similar), bestSim) {
					// Low similarity, update metadata and skip
					if _, err := m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
						ID:              a.ID,
						WorkspaceID:     wsID,
						ScoringMetadata: jsonBytes(map[string]any{"stage": "stage2_low_similarity", "best_similarity": bestSim, "auto_scored": true}),
						AwarenessLevel:  pgtype.Text{},
					}); err != nil {
						m.logger.Error().Err(err).Str("mention_id", a.ID).Msg("scorer: failed to record stage2 filter")
					}
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

	// Stage 3: Intent classification (bounded parallel LLM calls)
	m.forEachConcurrent(ctx, len(candidates), func(i int) {
		m.classifyAndStore(ctx, wsID, candidates[i].alert, candidates[i].similarity)
	})

	m.logger.Info().
		Int("total", len(alerts)).
		Int("scored", len(candidates)).
		Str("workspace_id", wsID).
		Msg("scorer: batch scoring complete")
}

// backfillUnclassified picks up mentions with intent IS NULL that were inserted
// in earlier ticks but never scored (usually because no AI provider was configured
// at the time). It converts them to mentionAlerts and runs them through the same
// batch scoring pipeline.
func (m *Monitor) backfillUnclassified(ctx context.Context, wsID string) {
	unclassified, err := m.q.ListUnclassifiedMentions(ctx, database.ListUnclassifiedMentionsParams{
		WorkspaceID: wsID,
		Lim:         50,
	})
	if err != nil {
		m.logger.Error().Err(err).Str("workspace_id", wsID).Msg("backfill: failed to list unclassified mentions")
		return
	}
	if len(unclassified) == 0 {
		return
	}

	m.logger.Info().
		Int("count", len(unclassified)).
		Str("workspace_id", wsID).
		Msg("backfill: scoring previously unclassified mentions")

	alerts := make([]mentionAlert, 0, len(unclassified))
	for _, u := range unclassified {
		alerts = append(alerts, mentionAlert{
			ID:          u.ID,
			WorkspaceID: u.WorkspaceID,
			Platform:    string(u.Platform),
			Title:       u.Title.String,
			URL:         u.Url,
			Content:     u.Content,
			Author:      u.AuthorUsername.String,
		})
	}
	m.batchScoreMentions(ctx, wsID, alerts)
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
		// A duplicate idempotency key just means this event was already
		// emitted (e.g. the mention is being re-scored). Not an error.
		if errors.Is(err, publishers.ErrDuplicateEvent) {
			m.logger.Debug().Str("mention_id", alert.ID).Msg("streams: mention.scored already published")
		} else {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish mention.scored")
		}
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
		if errors.Is(err, publishers.ErrDuplicateEvent) {
			m.logger.Debug().Str("mention_id", alert.ID).Msg("streams: notification request already published")
		} else {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish notification request")
		}
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
		// A duplicate idempotency key just means this event was already
		// emitted (e.g. the mention is being re-scored). Not an error.
		if errors.Is(err, publishers.ErrDuplicateEvent) {
			m.logger.Debug().Str("mention_id", alert.ID).Msg("streams: workflow trigger already published")
		} else {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish workflow trigger")
		}
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

// scoreStage1Rules is a zero-cost rules pre-filter (Stage 0). Returns true if the mention should proceed.
// minPainPointSimilarity is the cosine similarity a post needs to the nearest
// embedded pain point to reach classification.
const minPainPointSimilarity = 0.40

// lowPainPointSimilarity: skip only when the workspace has embedded pain
// points to compare against and the nearest is too far. No embedded pain
// points (or a failed lookup) means classify everything, never drop it all.
func lowPainPointSimilarity(matches int, best float64) bool {
	return matches > 0 && best < minPainPointSimilarity
}

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

func (m *Monitor) markStageFiltered(ctx context.Context, alert mentionAlert, stage, reason string) {
	_, _ = m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
		ID:          alert.ID,
		WorkspaceID: alert.WorkspaceID,
		ScoringMetadata: jsonBytes(map[string]any{
			"stage":       stage,
			"auto_scored": true,
			"reason":      reason,
			"filtered":    true,
		}),
		AwarenessLevel: pgtype.Text{},
	})
}

// qualifyAsLead auto-creates a lead for high-intent mentions.
func (m *Monitor) qualifyAsLead(ctx context.Context, alert mentionAlert, result *ai.ClassifyResult) {
	lead, err := m.q.CreateLead(ctx, database.CreateLeadParams{
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

	if m.enricher != nil {
		m.enricher.EnrichLeadAsync(lead.ID, alert.WorkspaceID)
	}
	m.crm.Emit(alert.WorkspaceID, crmsync.Event{Type: crmsync.EventLeadCreated, LeadID: lead.ID})

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
		// A duplicate idempotency key just means this event was already
		// emitted (e.g. the mention is being re-scored). Not an error.
		if errors.Is(err, publishers.ErrDuplicateEvent) {
			m.logger.Debug().Str("mention_id", alert.ID).Msg("streams: mention.qualified already published")
		} else {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish mention.qualified")
		}
	}
}

// classifyAndStore runs stage 3 (intent) and stage 4 (lead qualification) for one mention.
func (m *Monitor) classifyAndStore(ctx context.Context, wsID string, a mentionAlert, similarity float64) {
	c := struct {
		alert      mentionAlert
		similarity float64
	}{a, similarity}
	result, err := m.llmRouter.ClassifyIntent(ctx, wsID, c.alert.Title, c.alert.Content, c.alert.Platform, m.scoringContextFor(ctx, wsID, c.alert.ID))
	if err != nil {
		// Record the failure. Without this the mention keeps null scoring
		// metadata, which is indistinguishable from "never reached stage 3":
		// it is not retried, not surfaced as filtered, and silently stalls.
		m.logger.Error().Err(err).Str("mention_id", c.alert.ID).Msg("scorer: classification failed")
		m.markStageFiltered(ctx, c.alert, "stage3_classification_error", err.Error())
		return
	}
	// scoring: normalize whatever the scorer returned (idempotent for the router)
	normalized := ai.NormalizeClassifyResult(*result, c.alert.Title)
	result = &normalized

	meta := map[string]any{
		"stage":           "stage3_classified",
		"best_similarity": c.similarity,
		"auto_scored":     true,
		"reasoning":       result.Reasoning,
		"awareness_level": result.AwarenessLevel,
		// scoring: taxonomy context
		"intent_tags":   result.IntentTags,
		"sentiment":     result.Sentiment,
		"post_type":     result.PostType,
		"matches_skips": result.MatchesSkips,
	}

	// Update mention with classification + awareness level
	if _, err := m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
		ID:                    c.alert.ID,
		WorkspaceID:           c.alert.WorkspaceID,
		Intent:                database.NullIntentType{IntentType: database.IntentType(result.Intent), Valid: true},
		ConversionProbability: pgtype.Float4{Float32: float32(result.ConversionProbability), Valid: true},
		RelevanceScore:        pgtype.Float4{Float32: float32(result.RelevanceScore), Valid: true},
		ScoringMetadata:       jsonBytes(meta),
		AwarenessLevel:        pgtype.Text{String: result.AwarenessLevel, Valid: result.AwarenessLevel != ""},
	}); err != nil {
		m.logger.Error().Err(err).Str("mention_id", c.alert.ID).Msg("scorer: failed to persist classification")
	}

	m.storeIntentTags(ctx, c.alert.WorkspaceID, c.alert.ID, result) // scoring
	m.publishMentionScored(ctx, c.alert, result)

	// Stage 4: Lead qualification (inline unless async qualifier consumer is enabled)
	if !m.qualifierAsync && qualifiesAsLead(result.RelevanceScore, result.Intent) {
		m.qualifyAsLead(ctx, c.alert, result)
	}
	m.runWorkflows(ctx, c.alert, result)
}

// runWorkflows runs the workspace's real-time workflows inline, unless the
// streams pipeline is on (publishWorkflowTrigger hands the lead to its consumer).
func (m *Monitor) runWorkflows(ctx context.Context, a mentionAlert, r *ai.ClassifyResult) {
	if m.workflows == nil || (m.streamsEnabled && m.eventPublisher != nil) {
		return
	}
	m.workflows.Run(ctx, workflow.Lead{
		MentionID: a.ID, WorkspaceID: a.WorkspaceID, Platform: a.Platform, Keyword: a.Keyword,
		Title: a.Title, Content: a.Content, URL: a.URL, Author: a.Author,
		Intent: r.Intent, IntentTags: r.IntentTags, Score: r.RelevanceScore,
	})
}

// leadScoreThreshold must match the leads_ready SQL predicate in
// database/mentions_composed.go and queries/mentions.sql.
const leadScoreThreshold = 6.0

// qualifiesAsLead: scores are product-aware (competitor launches score 0-3),
// so 6+ with buying-type intent is a real prospect. comparison counts: someone
// weighing options is shopping.
func qualifiesAsLead(score float64, intent string) bool {
	if score < leadScoreThreshold {
		return false
	}
	switch database.IntentType(intent) {
	case database.IntentTypeBuySignal, database.IntentTypeRecommendationAsk,
		database.IntentTypeComplaint, database.IntentTypeComparison:
		return true
	}
	return false
}

// scoringConcurrency bounds parallel LLM calls per scoring batch.
// ponytail: fixed at 4 (safe under typical provider rate limits). Make it a
// config knob if a provider throttles or a faster plan allows more.
const scoringConcurrency = 4

// forEachConcurrent calls fn(0..n-1) with at most scoringConcurrency in flight
// and returns when all are done. Stops launching new work once ctx is cancelled.
func (m *Monitor) forEachConcurrent(ctx context.Context, n int, fn func(i int)) {
	sem := make(chan struct{}, scoringConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() {
				// The tick-level recover can't see panics in these goroutines;
				// without this one bad mention would crash the process.
				if r := recover(); r != nil {
					m.logger.Error().Interface("panic", r).Int("index", i).Msg("scorer: recovered from panic in worker")
				}
				<-sem
				wg.Done()
			}()
			fn(i)
		}(i)
	}
	wg.Wait()
}
