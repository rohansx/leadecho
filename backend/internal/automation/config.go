package automation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	KindDailyTriage       = "daily_triage"
	KindLeadSweep         = "lead_sweep"
	KindAttributionReview = "attribution_review"
	KindCustom            = "custom"

	maxNameLen  = 200
	maxSteps    = 5
	maxArgsSize = 4 << 10
)

var kinds = map[string]bool{KindDailyTriage: true, KindLeadSweep: true, KindAttributionReview: true, KindCustom: true}

// Config tunes a playbook; only the fields of its kind are used.
type Config struct {
	TopN        int     `json:"top_n,omitempty"`        // daily_triage: list size
	MarkNoise   bool    `json:"mark_noise,omitempty"`   // daily_triage: archive obvious noise
	NoiseBelow  float32 `json:"noise_below,omitempty"`  // daily_triage: noise = score below this
	MinScore    float32 `json:"min_score,omitempty"`    // lead_sweep: minimum relevance score
	MaxLeads    int     `json:"max_leads,omitempty"`    // lead_sweep: cap per run
	PushToCRM   bool    `json:"push_to_crm,omitempty"`  // lead_sweep: sync new leads to the CRM
	Days        int     `json:"days,omitempty"`         // attribution_review: window
	MinMentions int     `json:"min_mentions,omitempty"` // attribution_review: mute-suggestion threshold
	Email       bool    `json:"email,omitempty"`        // also email the output to the owner
	Steps       []Step  `json:"steps,omitempty"`        // custom: read-only tool calls
}

// Step is one tool call of a custom automation.
type Step struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Input is a create/update request.
type Input struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Enabled  bool     `json:"enabled"`
	Schedule Schedule `json:"schedule"`
	Config   Config   `json:"config"`
}

// Normalize validates the request and fills per-kind defaults.
func (in Input) Normalize() (Input, error) {
	out := in
	out.Name = strings.TrimSpace(in.Name)
	if out.Name == "" || utf8.RuneCountInString(out.Name) > maxNameLen {
		return Input{}, fmt.Errorf("name is required (at most %d characters)", maxNameLen)
	}
	if !kinds[in.Kind] {
		return Input{}, errors.New("kind must be daily_triage, lead_sweep, attribution_review or custom")
	}
	var err error
	if out.Schedule, err = in.Schedule.Normalize(); err != nil {
		return Input{}, err
	}
	out.Config, err = in.Config.normalize(in.Kind)
	return out, err
}

func (c Config) normalize(kind string) (Config, error) {
	out := Config{Email: c.Email}
	switch kind {
	case KindDailyTriage:
		if c.NoiseBelow < 0 || c.NoiseBelow > 10 {
			return Config{}, errors.New("noise_below must be between 0 and 10")
		}
		out.TopN = clamp(c.TopN, 10, 50)
		out.MarkNoise = c.MarkNoise
		out.NoiseBelow = c.NoiseBelow
		if out.NoiseBelow == 0 {
			out.NoiseBelow = 3
		}
	case KindLeadSweep:
		if c.MinScore < 0 || c.MinScore > 10 {
			return Config{}, errors.New("min_score must be between 0 and 10")
		}
		out.MinScore = c.MinScore
		if out.MinScore == 0 {
			out.MinScore = 6
		}
		out.MaxLeads = clamp(c.MaxLeads, 20, 100)
		out.PushToCRM = c.PushToCRM
	case KindAttributionReview:
		out.Days = clamp(c.Days, 7, 90)
		out.MinMentions = clamp(c.MinMentions, 5, 1000)
	case KindCustom:
		steps, err := normalizeSteps(c.Steps)
		if err != nil {
			return Config{}, err
		}
		out.Steps = steps
	}
	return out, nil
}

func normalizeSteps(in []Step) ([]Step, error) {
	if len(in) == 0 || len(in) > maxSteps {
		return nil, fmt.Errorf("custom automations need 1 to %d steps", maxSteps)
	}
	out := make([]Step, len(in))
	for i, s := range in {
		name := strings.TrimSpace(s.Tool)
		if name == "" {
			return nil, fmt.Errorf("step %d: tool is required", i+1)
		}
		args := bytes.TrimSpace(s.Args)
		if len(args) == 0 || bytes.Equal(args, []byte("null")) {
			args = []byte("{}")
		}
		var obj map[string]any
		if len(args) > maxArgsSize || json.Unmarshal(args, &obj) != nil {
			return nil, fmt.Errorf("step %d: args must be a JSON object (at most %d bytes)", i+1, maxArgsSize)
		}
		out[i] = Step{Tool: name, Args: json.RawMessage(args)}
	}
	return out, nil
}

// clamp applies def when v is unset and caps v at max.
func clamp(v, def, max int) int {
	switch {
	case v <= 0:
		return def
	case v > max:
		return max
	}
	return v
}
