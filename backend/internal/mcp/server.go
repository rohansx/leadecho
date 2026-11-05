// Package mcp is a small hand-rolled Model Context Protocol server over the
// Streamable HTTP transport (POST, JSON responses, stateless — no SSE stream).
// It exposes the tools.Registry to Claude, ChatGPT, Cursor and other MCP clients.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/rs/zerolog"

	"leadecho/internal/apikey"
	"leadecho/internal/tools"
)

const (
	LatestProtocolVersion = "2025-06-18"
	maxBody               = 1 << 20

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602

	instructions = "LeadEcho finds buying-intent social mentions, drafts knowledge-base-grounded replies and attributes revenue to them. Start with weekly_summary or search_mentions (tier=leads_ready), then draft_reply and queue_reply; use set_stage and push_to_crm to work the pipeline."
)

// supportedVersions are echoed back when a client asks for them; anything
// else is answered with the latest version.
var supportedVersions = []string{LatestProtocolVersion, "2025-03-26", "2024-11-05"}

// Server handles POST /mcp. Auth returns the caller's workspace and API key
// scopes (set by the API key middleware upstream).
type Server struct {
	Registry *tools.Registry
	Version  string
	Logger   zerolog.Logger
	Auth     func(ctx context.Context) (workspaceID string, scopes []string)
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

var nullID = json.RawMessage("null")

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	ws, scopes := s.Auth(r.Context())
	if ws == "" {
		http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !slices.Contains(supportedVersions, v) {
		writeRPC(w, http.StatusBadRequest, nullID, nil, &rpcError{codeInvalidRequest, "unsupported MCP-Protocol-Version: " + v})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeRPC(w, http.StatusBadRequest, nullID, nil, &rpcError{codeInvalidRequest, "request body too large"})
		return
	}
	body = bytes.TrimSpace(body)
	if len(body) > 0 && body[0] == '[' { // batching was removed in 2025-06-18
		writeRPC(w, http.StatusBadRequest, nullID, nil, &rpcError{codeInvalidRequest, "batch requests are not supported"})
		return
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPC(w, http.StatusBadRequest, nullID, nil, &rpcError{codeParseError, "parse error"})
		return
	}
	id := req.ID
	if len(id) == 0 {
		id = nullID
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		writeRPC(w, http.StatusBadRequest, id, nil, &rpcError{codeInvalidRequest, "invalid request"})
		return
	}
	if len(req.ID) == 0 { // notification (e.g. notifications/initialized): nothing to answer
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, rpcErr := s.dispatch(r.Context(), ws, scopes, req)
	writeRPC(w, http.StatusOK, id, result, rpcErr)
}

func (s *Server) dispatch(ctx context.Context, ws string, scopes []string, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params), nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return s.listTools(scopes), nil
	case "tools/call":
		return s.callTool(ctx, ws, scopes, req.Params)
	default:
		return nil, &rpcError{codeMethodNotFound, "method not found: " + req.Method}
	}
}

func (s *Server) initialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p) // a missing/garbled version just gets the latest
	v := LatestProtocolVersion
	if slices.Contains(supportedVersions, p.ProtocolVersion) {
		v = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": v,
		"capabilities":    map[string]any{"tools": map[string]bool{"listChanged": false}},
		"serverInfo":      map[string]string{"name": "leadecho", "title": "LeadEcho", "version": s.Version},
		"instructions":    instructions,
	}
}

type toolDesc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations map[string]bool `json:"annotations"`
}

func (s *Server) listTools(scopes []string) any {
	out := []toolDesc{}
	for _, t := range s.Registry.Allowed(scopes) {
		out = append(out, toolDesc{
			Name: t.Name, Description: t.Description, InputSchema: t.InputSchema,
			Annotations: map[string]bool{"readOnlyHint": t.Scope == apikey.ScopeRead},
		})
	}
	return map[string]any{"tools": out}
}

func (s *Server) callTool(ctx context.Context, ws string, scopes []string, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, &rpcError{codeInvalidParams, "invalid params: name is required"}
	}
	out, err := s.Registry.Call(ctx, ws, scopes, p.Name, p.Arguments)
	var ue *tools.Error
	switch {
	case errors.Is(err, tools.ErrUnknownTool):
		return nil, &rpcError{codeInvalidParams, "unknown tool: " + p.Name}
	case errors.As(err, &ue):
		return toolError(ue.Msg), nil
	case err != nil:
		s.Logger.Error().Err(err).Str("tool", p.Name).Str("workspace_id", ws).Msg("mcp tool failed")
		return toolError("internal error running " + p.Name), nil
	}
	text, err := json.Marshal(out)
	if err != nil {
		s.Logger.Error().Err(err).Str("tool", p.Name).Msg("mcp tool result not encodable")
		return toolError("internal error running " + p.Name), nil
	}
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(text)}},
		"structuredContent": out,
		"isError":           false,
	}, nil
}

func toolError(msg string) any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": msg}}, "isError": true}
}

func writeRPC(w http.ResponseWriter, status int, id json.RawMessage, result any, rpcErr *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr})
}
