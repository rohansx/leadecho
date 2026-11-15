package monitor

import (
	"errors"
	"fmt"
	"testing"

	"leadecho/internal/ai"
)

// A refusal is final; anything else (an outage, a bad parse) stays retryable.
func TestClassifyErrorStage(t *testing.T) {
	refused := fmt.Errorf("anthropic: %w (cyber)", ai.ErrRefused)
	if got := classifyErrorStage(refused); got != "stage3_refused" {
		t.Errorf("refusal: got %q", got)
	}
	if got := classifyErrorStage(errors.New("503 Service Unavailable")); got != "stage3_classification_error" {
		t.Errorf("outage: got %q", got)
	}
}
