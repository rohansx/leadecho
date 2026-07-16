package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	SchemaVersionV1 = 1
)

const (
	EventTypeMentionIngested            = "mention.ingested.v1"
	EventTypeMentionScored              = "mention.scored.v1"
	EventTypeMentionQualified           = "mention.qualified.v1"
	EventTypeMentionNotificationRequest = "mention.notification_requested.v1"
	EventTypeReplyDraftRequested        = "reply.draft_requested.v1"
	EventTypeReplyApproved              = "reply.approved.v1"
	EventTypeWorkflowTriggerRequested   = "workflow.trigger_requested.v1"
)

const (
	AggregateTypeMention  = "mention"
	AggregateTypeReply    = "reply"
	AggregateTypeWorkflow = "workflow"
)

type Envelope struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	SchemaVersion  int32           `json:"schema_version"`
	WorkspaceID    string          `json:"workspace_id,omitempty"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	Producer       string          `json:"producer"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	CausationID    string          `json:"causation_id,omitempty"`
	TraceID        string          `json:"trace_id,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
}

func (e Envelope) Validate() error {
	switch {
	case e.EventID == "":
		return fmt.Errorf("event_id is required")
	case e.EventType == "":
		return fmt.Errorf("event_type is required")
	case e.SchemaVersion <= 0:
		return fmt.Errorf("schema_version must be positive")
	case e.AggregateType == "":
		return fmt.Errorf("aggregate_type is required")
	case e.AggregateID == "":
		return fmt.Errorf("aggregate_id is required")
	case e.Producer == "":
		return fmt.Errorf("producer is required")
	case len(e.Payload) == 0:
		return fmt.Errorf("payload is required")
	case e.IdempotencyKey == "":
		return fmt.Errorf("idempotency_key is required")
	case e.OccurredAt.IsZero():
		return fmt.Errorf("occurred_at is required")
	default:
		return nil
	}
}

func NewEnvelope(eventType, aggregateType, aggregateID, producer, workspaceID, idempotencyKey string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal payload: %w", err)
	}
	id, err := NewEventID()
	if err != nil {
		return Envelope{}, err
	}
	env := Envelope{
		EventID:        id,
		EventType:      eventType,
		SchemaVersion:  SchemaVersionV1,
		WorkspaceID:    workspaceID,
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		Producer:       producer,
		Payload:        raw,
		IdempotencyKey: idempotencyKey,
		OccurredAt:     time.Now().UTC(),
	}
	return env, env.Validate()
}

func NewEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:]), nil
}

type MentionIngestedPayload struct {
	MentionID   string `json:"mention_id"`
	WorkspaceID string `json:"workspace_id"`
	Platform    string `json:"platform"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url"`
	Author      string `json:"author,omitempty"`
	Content     string `json:"content"`
	Keyword     string `json:"keyword,omitempty"`
	Source      string `json:"source,omitempty"`
}

type MentionScoredPayload struct {
	MentionID              string   `json:"mention_id"`
	WorkspaceID            string   `json:"workspace_id"`
	Platform               string   `json:"platform"`
	Title                  string   `json:"title,omitempty"`
	URL                    string   `json:"url"`
	Author                 string   `json:"author,omitempty"`
	Content                string   `json:"content"`
	Intent                 string   `json:"intent,omitempty"`
	AwarenessLevel         string   `json:"awareness_level,omitempty"`
	RelevanceScore         float32  `json:"relevance_score,omitempty"`
	ConversionProbability  float32  `json:"conversion_probability,omitempty"`
	ScoringStage           string   `json:"scoring_stage,omitempty"`
	NotificationCandidates []string `json:"notification_candidates,omitempty"`
}

type MentionNotificationRequestedPayload struct {
	MentionID   string  `json:"mention_id"`
	WorkspaceID string  `json:"workspace_id"`
	Platform    string  `json:"platform"`
	Keyword     string  `json:"keyword,omitempty"`
	Title       string  `json:"title,omitempty"`
	URL         string  `json:"url"`
	Author      string  `json:"author,omitempty"`
	Score       float32 `json:"score,omitempty"`
}

type MentionQualifiedPayload struct {
	MentionID   string  `json:"mention_id"`
	WorkspaceID string  `json:"workspace_id"`
	Intent      string  `json:"intent"`
	Score       float32 `json:"score"`
}
