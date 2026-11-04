// Package tools is LeadEcho's transport-agnostic agent tool layer. The MCP
// server, the public REST API and the in-app AI chat all call the same
// Registry, so a tool behaves identically everywhere.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/reply"
)

// Handler runs a tool for one workspace. args is the raw JSON object sent by
// the caller; the result must be JSON-encodable.
type Handler func(ctx context.Context, workspaceID string, args json.RawMessage) (any, error)

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema (object)
	Scope       string          // API key scope required to call it
	Handler     Handler
}

// Error is a caller-facing failure (bad arguments, not found, forbidden). Its
// message is safe to show; any other error is internal and must not leak.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func userErr(format string, a ...any) error { return &Error{Msg: fmt.Sprintf(format, a...)} }

var ErrUnknownTool = errors.New("unknown tool")

// Drafter drafts a KB-grounded reply for a mention (reply.Drafter).
type Drafter interface {
	DraftForMention(ctx context.Context, wsID, mentionID string) (*reply.DraftResult, error)
}

// CRMEmitter queues an async CRM/webhook sync (crmsync.Dispatcher).
type CRMEmitter interface {
	Emit(wsID string, ev crmsync.Event)
}

// Mute is one mute-list entry.
type Mute struct {
	Kind  string `json:"kind"` // author | domain | keyword | subreddit ("topic" = keyword)
	Value string `json:"value"`
}

// MuteStore persists mute-list entries; *mutes.Store satisfies it.
type MuteStore interface {
	Add(ctx context.Context, workspaceID, kind, value string) error
}

// Deps are the services tools are built on. Drafter, CRM and Mutes may be nil:
// the tools that need them then answer with a clear "not available" error.
type Deps struct {
	Q       database.Querier
	Drafter Drafter
	CRM     CRMEmitter
	Mutes   MuteStore
}

type Registry struct {
	tools []Tool
	index map[string]int
}

// New builds the registry with every LeadEcho tool.
func New(d Deps) *Registry {
	r := &Registry{index: map[string]int{}}
	for _, t := range append(readTools(d), writeTools(d)...) {
		r.index[t.Name] = len(r.tools)
		r.tools = append(r.tools, t)
	}
	return r
}

// List returns the tools in a stable order.
func (r *Registry) List() []Tool { return slices.Clone(r.tools) }

// Allowed returns the tools callable with scopes.
func (r *Registry) Allowed(scopes []string) []Tool {
	var out []Tool
	for _, t := range r.tools {
		if slices.Contains(scopes, t.Scope) {
			out = append(out, t)
		}
	}
	return out
}

// Call validates scope and workspace, then runs the tool.
func (r *Registry) Call(ctx context.Context, workspaceID string, scopes []string, name string, args json.RawMessage) (any, error) {
	i, ok := r.index[name]
	if !ok {
		return nil, ErrUnknownTool
	}
	t := r.tools[i]
	if !slices.Contains(scopes, t.Scope) {
		return nil, userErr("tool %s requires the %s scope", name, t.Scope)
	}
	if workspaceID == "" {
		return nil, userErr("no workspace in context")
	}
	return t.Handler(ctx, workspaceID, args)
}

// decode strictly parses args into v: unknown fields and wrong types fail.
func decode(args json.RawMessage, v any) error {
	args = bytes.TrimSpace(args)
	if len(args) == 0 || bytes.Equal(args, []byte("null")) {
		args = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return userErr("invalid arguments: %v", err)
	}
	if dec.More() {
		return userErr("invalid arguments: trailing data")
	}
	return nil
}
