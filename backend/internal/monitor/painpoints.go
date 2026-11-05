package monitor

import (
	"context"
	"encoding/json"
	"strings"

	"leadecho/internal/database"
	"leadecho/internal/llm"
)

// ensurePainPointEmbeddings self-heals semantic matching. Pain-point phrases
// live only in pain_point_embeddings, so a workspace onboarded while no
// embedding provider worked has none. Once one works, rebuild them from the
// brand profile. Checked once per workspace per process; a failed embed is
// retried on the next batch.
func (m *Monitor) ensurePainPointEmbeddings(ctx context.Context, wsID string) {
	if m.llmRouter == nil {
		return
	}
	if _, done := m.painPointsChecked.LoadOrStore(wsID, struct{}{}); done {
		return
	}
	if existing, err := m.q.ListPainPointEmbeddingsByWorkspace(ctx, wsID); err != nil || len(existing) > 0 {
		return
	}
	profiles, err := m.q.ListActiveMonitoringProfiles(ctx, wsID)
	if err != nil || len(profiles) == 0 {
		return
	}
	bp, err := m.q.GetBrandProfile(ctx, wsID)
	if err != nil {
		return
	}
	phrases := brandPainPhrases(bp.PainPoints)
	if len(phrases) == 0 {
		return
	}
	vectors, err := m.llmRouter.EmbedTexts(ctx, wsID, llm.TaskEmbedProfiles, phrases)
	if err != nil {
		m.painPointsChecked.Delete(wsID) // retry once an embedding provider works
		m.logger.Debug().Err(err).Str("workspace_id", wsID).Msg("pain points: backfill embed failed")
		return
	}
	stored := 0
	for i, phrase := range phrases {
		if i >= len(vectors) {
			break
		}
		if _, err := m.q.CreatePainPointEmbedding(ctx, database.CreatePainPointEmbeddingParams{
			ProfileID: profiles[0].ID, WorkspaceID: wsID, Phrase: phrase, Embedding: &vectors[i],
		}); err == nil {
			stored++
		}
	}
	m.logger.Info().Int("count", stored).Str("workspace_id", wsID).Msg("pain points: rebuilt embeddings from brand profile")
}

// brandPainPhrases reads brand_profiles.pain_points: [{"problem": …}] rows,
// or plain strings from older rows.
func brandPainPhrases(raw []byte) []string {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	var out []string
	for _, r := range rows {
		var obj struct {
			Problem string `json:"problem"`
		}
		var s string
		if json.Unmarshal(r, &obj) == nil && obj.Problem != "" {
			s = obj.Problem
		} else if json.Unmarshal(r, &s) != nil {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
