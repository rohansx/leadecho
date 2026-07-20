package handler

import (
	"encoding/json"
	"time"
)

// mergeScoringFeedback overlays a human-feedback record onto existing scoring_metadata.
// Preserves prior keys so automated scoring context is not lost.
func mergeScoringFeedback(existing []byte, label, reason, actorID string, previousScore *float32, previousIntent string) ([]byte, error) {
	meta := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &meta)
	}

	entry := map[string]any{
		"label":      label,
		"reason":     reason,
		"actor_id":   actorID,
		"recorded_at": time.Now().UTC().Format(time.RFC3339),
	}
	if previousScore != nil {
		entry["previous_score"] = *previousScore
	}
	if previousIntent != "" {
		entry["previous_intent"] = previousIntent
	}

	history, _ := meta["feedback_history"].([]any)
	history = append(history, entry)
	// Cap history to keep row size bounded.
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	meta["feedback_history"] = history
	meta["last_feedback"] = entry

	return json.Marshal(meta)
}
