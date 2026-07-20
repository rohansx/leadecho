package handler

import (
	"encoding/json"
	"testing"
)

func TestMergeScoringFeedbackPreservesHistory(t *testing.T) {
	existing, _ := json.Marshal(map[string]any{"similarity": 0.72})
	score := float32(8.5)
	out, err := mergeScoringFeedback(existing, "spam", "promo thread", "user-1", &score, "general")
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(out, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["similarity"] != 0.72 {
		t.Fatalf("expected prior similarity preserved, got %#v", meta["similarity"])
	}
	last, ok := meta["last_feedback"].(map[string]any)
	if !ok || last["label"] != "spam" {
		t.Fatalf("missing last_feedback: %#v", meta["last_feedback"])
	}
	history, ok := meta["feedback_history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %#v", meta["feedback_history"])
	}
}
