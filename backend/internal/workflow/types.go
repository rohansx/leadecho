package workflow

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Trigger filters which newly scored leads start a workflow. Empty lists match
// everything; non-empty lists are "any of".
type Trigger struct {
	MinScore        float64  `json:"min_score"`
	Platforms       []string `json:"platforms,omitempty"`
	IntentTags      []string `json:"intent_tags,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`

	// Legacy v1 field (enum intents); still honoured when IntentTags is empty.
	IntentTypes []string `json:"intent_types,omitempty"`
}

// TriggerConfig is the v1 name, kept for the streams consumer.
type TriggerConfig = Trigger

type ActionType string

const (
	ActionNotifySlack   ActionType = "notify_slack"
	ActionNotifyDiscord ActionType = "notify_discord"
	ActionNotifyEmail   ActionType = "notify_email"
	ActionAIDraft       ActionType = "ai_draft"
	ActionCreateLead    ActionType = "create_lead" // config.tag (optional)
	ActionArchive       ActionType = "archive"
	ActionWebhook       ActionType = "webhook" // config.url (https, public)
	ActionPushCRM       ActionType = "push_crm"
)

var knownActions = map[ActionType]bool{
	ActionNotifySlack: true, ActionNotifyDiscord: true, ActionNotifyEmail: true, ActionAIDraft: true,
	ActionCreateLead: true, ActionArchive: true, ActionWebhook: true, ActionPushCRM: true,
}

type ActionConfig struct {
	Type   ActionType     `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

func (a ActionConfig) str(key string) string {
	s, _ := a.Config[key].(string)
	return strings.TrimSpace(s)
}

// StepResult is one action's outcome in a run.
type StepResult struct {
	Step   int        `json:"step"`
	Type   ActionType `json:"type"`
	Status string     `json:"status"` // completed | failed | skipped
	Detail string     `json:"detail,omitempty"`
	LeadID string     `json:"lead_id,omitempty"`
}

// Lead is a freshly scored post, the input every workflow sees.
type Lead struct {
	MentionID   string   `json:"mention_id"`
	WorkspaceID string   `json:"-"`
	Platform    string   `json:"platform"`
	Keyword     string   `json:"keyword,omitempty"`
	Title       string   `json:"title,omitempty"`
	Content     string   `json:"content"`
	URL         string   `json:"url"`
	Author      string   `json:"author,omitempty"`
	Intent      string   `json:"intent,omitempty"`
	IntentTags  []string `json:"intent_tags,omitempty"`
	Score       float64  `json:"score"`
}

// Matches reports whether the lead passes every filter of t.
func (t Trigger) Matches(l Lead) bool {
	if t.MinScore > 0 && l.Score < t.MinScore {
		return false
	}
	if len(t.Platforms) > 0 && !contains(t.Platforms, l.Platform) {
		return false
	}
	switch {
	case len(t.IntentTags) > 0:
		if !anyIn(t.IntentTags, l.IntentTags) {
			return false
		}
	case len(t.IntentTypes) > 0:
		if !contains(t.IntentTypes, l.Intent) {
			return false
		}
	}
	text := strings.ToLower(l.Title + " " + l.Content)
	if len(t.Keywords) > 0 && !containsAny(text, t.Keywords) {
		return false
	}
	return !(len(t.ExcludeKeywords) > 0 && containsAny(text, t.ExcludeKeywords))
}

const (
	maxNameLen   = 120
	maxActions   = 6
	maxListItems = 30
	// DefaultPerHour caps runs per workflow per hour so a viral thread can't
	// spend a workspace's credits or flood a Slack channel.
	DefaultPerHour = 20
	maxPerHour     = 200
)

// Definition is a workflow as the API accepts it.
type Definition struct {
	Name       string         `json:"name"`
	Enabled    bool           `json:"enabled"`
	Trigger    Trigger        `json:"trigger"`
	Actions    []ActionConfig `json:"actions"`
	MaxPerHour int            `json:"max_per_hour"`
}

// Normalize trims and validates d in place-free style, returning the cleaned copy.
func (d Definition) Normalize() (Definition, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > maxNameLen {
		return d, fmt.Errorf("name must be 1-%d characters", maxNameLen)
	}
	if d.Trigger.MinScore < 0 || d.Trigger.MinScore > 10 {
		return d, errors.New("min_score must be between 0 and 10")
	}
	for _, l := range [][]string{d.Trigger.Platforms, d.Trigger.IntentTags, d.Trigger.Keywords, d.Trigger.ExcludeKeywords} {
		if len(l) > maxListItems {
			return d, fmt.Errorf("filters take at most %d items", maxListItems)
		}
	}
	d.Trigger.Keywords = cleanList(d.Trigger.Keywords)
	d.Trigger.ExcludeKeywords = cleanList(d.Trigger.ExcludeKeywords)
	d.Trigger.Platforms = cleanList(d.Trigger.Platforms)
	d.Trigger.IntentTags = cleanList(d.Trigger.IntentTags)
	d.Trigger.IntentTypes = nil
	if len(d.Actions) == 0 || len(d.Actions) > maxActions {
		return d, fmt.Errorf("add 1-%d actions", maxActions)
	}
	seen := map[ActionType]bool{}
	for _, a := range d.Actions {
		if !knownActions[a.Type] {
			return d, fmt.Errorf("unknown action %q", a.Type)
		}
		if seen[a.Type] {
			return d, fmt.Errorf("action %q is listed twice", a.Type)
		}
		seen[a.Type] = true
		if a.Type == ActionWebhook && !strings.HasPrefix(a.str("url"), "https://") {
			return d, errors.New("webhook action needs an https:// url")
		}
		if a.Type == ActionCreateLead && utf8.RuneCountInString(a.str("tag")) > 40 {
			return d, errors.New("lead tag must be at most 40 characters")
		}
	}
	if seen[ActionArchive] && (seen[ActionAIDraft] || seen[ActionCreateLead] || seen[ActionPushCRM]) {
		return d, errors.New("archive can't be combined with drafting or pipeline actions")
	}
	if d.MaxPerHour <= 0 {
		d.MaxPerHour = DefaultPerHour
	}
	if d.MaxPerHour > maxPerHour {
		d.MaxPerHour = maxPerHour
	}
	return d, nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || seen[k] || utf8.RuneCountInString(s) > 100 {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

func contains(items []string, v string) bool {
	for _, item := range items {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}

func anyIn(want, have []string) bool {
	for _, w := range want {
		if contains(have, w) {
			return true
		}
	}
	return false
}

func containsAny(lowerText string, terms []string) bool {
	for _, t := range terms {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" && strings.Contains(lowerText, t) {
			return true
		}
	}
	return false
}
