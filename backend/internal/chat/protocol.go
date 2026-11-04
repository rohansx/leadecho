// Package chat is the in-app assistant ("Ask LeadEcho", P2 #15): an agent
// loop over the LLM router with the internal/tools registry as its tools.
//
// The router has no native tool calling, so the model speaks a constrained
// JSON protocol: every reply is exactly one object, either
// {"tool":"<name>","args":{...}} or {"answer":"<markdown>"}.
package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Step is one parsed model reply: a tool call or the final answer.
type Step struct {
	Tool   string          `json:"tool"`
	Args   json.RawMessage `json:"args"`
	Answer string          `json:"answer"`
}

// ParseStep strictly parses a model reply. known reports valid tool names.
func ParseStep(raw string, known func(string) bool) (Step, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	if s == "" {
		return Step{}, errors.New("empty reply")
	}
	if !strings.HasPrefix(s, "{") {
		return Step{}, errors.New("reply must be a single JSON object")
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	var st Step
	if err := dec.Decode(&st); err != nil {
		return Step{}, fmt.Errorf("invalid JSON: %v", err)
	}
	if dec.More() {
		return Step{}, errors.New("trailing data after the JSON object")
	}
	st.Tool = strings.TrimSpace(st.Tool)
	st.Answer = strings.TrimSpace(st.Answer)
	if (st.Tool == "") == (st.Answer == "") {
		return Step{}, errors.New(`give exactly one of "tool" or "answer"`)
	}
	if st.Answer != "" {
		if len(bytes.TrimSpace(st.Args)) > 0 {
			return Step{}, errors.New(`"args" only goes with "tool"`)
		}
		return st, nil
	}
	if !known(st.Tool) {
		return Step{}, fmt.Errorf("unknown tool %q", st.Tool)
	}
	args := bytes.TrimSpace(st.Args)
	switch {
	case len(args) == 0 || bytes.Equal(args, []byte("null")):
		st.Args = json.RawMessage(`{}`)
	case args[0] != '{':
		return Step{}, errors.New(`"args" must be a JSON object`)
	default:
		st.Args = json.RawMessage(args)
	}
	return st, nil
}
