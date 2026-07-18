package events

import "testing"

func TestNewEnvelopeBuildsValidEnvelope(t *testing.T) {
	env, err := NewEnvelope(
		EventTypeMentionIngested,
		AggregateTypeMention,
		"mention-123",
		"monitor",
		"workspace-123",
		"mention.ingested:mention-123",
		MentionIngestedPayload{
			MentionID:   "mention-123",
			WorkspaceID: "workspace-123",
			Platform:    "reddit",
			URL:         "https://example.com/post",
			Content:     "Need a CRM alternative for our team right now.",
		},
	)
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	if env.EventID == "" {
		t.Fatal("expected event id to be generated")
	}
	if env.EventType != EventTypeMentionIngested {
		t.Fatalf("unexpected event type: %s", env.EventType)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEnvelopeValidateRejectsMissingFields(t *testing.T) {
	env := Envelope{}
	if err := env.Validate(); err == nil {
		t.Fatal("expected validation error for empty envelope")
	}
}
