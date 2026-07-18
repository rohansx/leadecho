package workflow

type TriggerConfig struct {
	Platforms   []string `json:"platforms"`
	MinScore    float64  `json:"min_score"`
	IntentTypes []string `json:"intent_types"`
	Keywords    []string `json:"keywords,omitempty"`
}

type ActionType string

const (
	ActionAIDraft       ActionType = "ai_draft"
	ActionNotifySlack   ActionType = "notify_slack"
	ActionNotifyDiscord ActionType = "notify_discord"
	ActionApprovalGate  ActionType = "approval_gate"
	ActionPostReply     ActionType = "post_reply"
	ActionCreateLead    ActionType = "create_lead"
	ActionWebhook       ActionType = "webhook"
	ActionDelay         ActionType = "delay"
	ActionCondition     ActionType = "condition"
	ActionTagMention    ActionType = "tag_mention"
)

type ActionConfig struct {
	Type   ActionType     `json:"type"`
	Config map[string]any `json:"config"`
}

type StepResult struct {
	Step     int        `json:"step"`
	Type     ActionType `json:"type"`
	Status   string     `json:"status"`
	Detail   string     `json:"detail,omitempty"`
	ReplyID  string     `json:"reply_id,omitempty"`
	LeadID   string     `json:"lead_id,omitempty"`
}
