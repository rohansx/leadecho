package events

const (
	StreamMentionEvents  = "leadecho:mention_events"
	StreamReplyEvents    = "leadecho:reply_events"
	StreamWorkflowEvents = "leadecho:workflow_events"
	StreamOpsDeadLetter  = "leadecho:ops_dead_letter"
	StreamOpsAudit       = "leadecho:ops_audit"
)

const (
	GroupMentionScorers    = "mention_scorers"
	GroupMentionQualifiers = "mention_qualifiers"
	GroupMentionNotifiers  = "mention_notifiers"
	GroupReplyDrafters     = "reply_drafters"
	GroupWorkflowExecutors = "workflow_executors"
	GroupOpsRetries        = "ops_retries"
)

func StreamForEventType(eventType string) string {
	switch eventType {
	case EventTypeMentionIngested, EventTypeMentionScored, EventTypeMentionQualified, EventTypeMentionNotificationRequest:
		return StreamMentionEvents
	case EventTypeReplyDraftRequested, EventTypeReplyApproved:
		return StreamReplyEvents
	case EventTypeWorkflowTriggerRequested:
		return StreamWorkflowEvents
	default:
		return StreamOpsAudit
	}
}
