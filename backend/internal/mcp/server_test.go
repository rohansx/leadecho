package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/tools"
)

const testWS = "11111111-1111-1111-1111-111111111111"

type fakeQ struct{ database.Querier }

func (fakeQ) ListLeads(_ context.Context, a database.ListLeadsParams) ([]database.Lead, error) {
	if a.WorkspaceID != testWS {
		return nil, errors.New("wrong workspace")
	}
	return []database.Lead{{ID: "lead-1", Stage: "qualified", Tags: []string{}, CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}}, nil
}
func (fakeQ) GetLead(context.Context, database.GetLeadParams) (database.Lead, error) {
	return database.Lead{}, pgx.ErrNoRows
}
func (fakeQ) ListMentions(context.Context, database.ListMentionsParams) ([]database.Mention, error) {
	return nil, errors.New("connection refused to 10.0.0.5")
}

func newServer(scopes ...string) *Server {
	return &Server{
		Registry: tools.New(tools.Deps{Q: fakeQ{}}),
		Version:  "1.2.3",
		Logger:   zerolog.Nop(),
		Auth:     func(context.Context) (string, []string) { return testWS, scopes },
	}
}

func post(t *testing.T, s *Server, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// jsonEq compares JSON documents structurally.
func jsonEq(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("response is not JSON: %q", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad golden: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if !bytes.Equal(gb, wb) {
		t.Fatalf("got  %s\nwant %s", gb, wb)
	}
}

func TestGolden(t *testing.T) {
	s := newServer("read")
	tests := []struct {
		name, body, want string
		status           int
	}{
		{
			"initialize latest",
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude","version":"1"}}}`,
			`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{"listChanged":false}},"serverInfo":{"name":"leadecho","title":"LeadEcho","version":"1.2.3"},"instructions":"` + instructions + `"}}`,
			200,
		},
		{
			"initialize older supported version is echoed",
			`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
			`{"jsonrpc":"2.0","id":"a","result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{"listChanged":false}},"serverInfo":{"name":"leadecho","title":"LeadEcho","version":"1.2.3"},"instructions":"` + instructions + `"}}`,
			200,
		},
		{
			"initialize unknown version gets latest",
			`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
			`{"jsonrpc":"2.0","id":2,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{"listChanged":false}},"serverInfo":{"name":"leadecho","title":"LeadEcho","version":"1.2.3"},"instructions":"` + instructions + `"}}`,
			200,
		},
		{"ping", `{"jsonrpc":"2.0","id":3,"method":"ping"}`, `{"jsonrpc":"2.0","id":3,"result":{}}`, 200},
		{"unknown method", `{"jsonrpc":"2.0","id":4,"method":"resources/list"}`, `{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"method not found: resources/list"}}`, 200},
		{"parse error", `{"jsonrpc":`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`, 400},
		{"wrong jsonrpc version", `{"jsonrpc":"1.0","id":5,"method":"ping"}`, `{"jsonrpc":"2.0","id":5,"error":{"code":-32600,"message":"invalid request"}}`, 400},
		{"missing method", `{"jsonrpc":"2.0","id":6}`, `{"jsonrpc":"2.0","id":6,"error":{"code":-32600,"message":"invalid request"}}`, 400},
		{"batch rejected", `[{"jsonrpc":"2.0","id":7,"method":"ping"}]`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch requests are not supported"}}`, 400},
		{
			"tools/call success",
			`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"list_leads","arguments":{"limit":1}}}`,
			`{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"{\"count\":1,\"leads\":[{\"id\":\"lead-1\",\"stage\":\"qualified\",\"tags\":[],\"created_at\":\"1970-01-01T00:00:00Z\",\"updated_at\":\"1970-01-01T00:00:00Z\"}],\"limit\":1,\"offset\":0}"}],"structuredContent":{"count":1,"leads":[{"id":"lead-1","stage":"qualified","tags":[],"created_at":"1970-01-01T00:00:00Z","updated_at":"1970-01-01T00:00:00Z"}],"limit":1,"offset":0},"isError":false}}`,
			200,
		},
		{
			"tools/call tool error is a result",
			`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_lead","arguments":{"lead_id":"99999999-9999-9999-9999-999999999999"}}}`,
			`{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"lead not found"}],"isError":true}}`,
			200,
		},
		{
			"tools/call bad arguments",
			`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"list_leads","arguments":{"nope":1}}}`,
			`{"jsonrpc":"2.0","id":10,"result":{"content":[{"type":"text","text":"invalid arguments: json: unknown field \"nope\""}],"isError":true}}`,
			200,
		},
		{
			"tools/call internal error does not leak",
			`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"search_mentions"}}`,
			`{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"internal error running search_mentions"}],"isError":true}}`,
			200,
		},
		{
			"tools/call missing scope",
			`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"set_stage","arguments":{}}}`,
			`{"jsonrpc":"2.0","id":12,"result":{"content":[{"type":"text","text":"tool set_stage requires the write scope"}],"isError":true}}`,
			200,
		},
		{
			"tools/call unknown tool",
			`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"rm_rf"}}`,
			`{"jsonrpc":"2.0","id":13,"error":{"code":-32602,"message":"unknown tool: rm_rf"}}`,
			200,
		},
		{
			"tools/call without name",
			`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{}}`,
			`{"jsonrpc":"2.0","id":14,"error":{"code":-32602,"message":"invalid params: name is required"}}`,
			200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, s, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.status, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type %q", ct)
			}
			jsonEq(t, rec.Body.Bytes(), tt.want)
		})
	}
}

func TestToolsListFiltersByScope(t *testing.T) {
	names := func(s *Server) []string {
		rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		var resp struct {
			Result struct {
				Tools []struct {
					Name        string          `json:"name"`
					Description string          `json:"description"`
					InputSchema json.RawMessage `json:"inputSchema"`
					Annotations struct {
						ReadOnlyHint bool `json:"readOnlyHint"`
					} `json:"annotations"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tl := range resp.Result.Tools {
			if tl.Description == "" || len(tl.InputSchema) == 0 {
				t.Fatalf("tool %s incomplete", tl.Name)
			}
			if tl.Name == "list_leads" && !tl.Annotations.ReadOnlyHint {
				t.Fatal("read tools must be readOnlyHint")
			}
			out = append(out, tl.Name)
		}
		return out
	}
	if got := names(newServer("read")); len(got) != 5 || strings.Contains(strings.Join(got, ","), "set_stage") {
		t.Fatalf("read key sees %v", got)
	}
	if got := names(newServer("read", "write")); len(got) != 12 {
		t.Fatalf("write key sees %d tools", len(got))
	}
}

func TestTransport(t *testing.T) {
	s := newServer("read")
	// Notifications get 202 with no body.
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
	} {
		rec := post(t, s, body)
		if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
			t.Fatalf("notification: %d %q", rec.Code, rec.Body)
		}
	}
	// GET (server-initiated SSE stream) is not offered.
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
		t.Fatalf("GET: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	// Unsupported MCP-Protocol-Version header is a 400; supported ones pass.
	if rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "MCP-Protocol-Version", "2000-01-01"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad version header: %d", rec.Code)
	}
	if rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "MCP-Protocol-Version", "2025-03-26"); rec.Code != http.StatusOK {
		t.Fatalf("supported version header: %d", rec.Code)
	}
	// Oversized bodies are refused.
	if rec := post(t, s, `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"`+strings.Repeat("a", maxBody)+`"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized: %d", rec.Code)
	}
	// No workspace from auth: refuse.
	noAuth := newServer()
	noAuth.Auth = func(context.Context) (string, []string) { return "", nil }
	if rec := post(t, noAuth, `{"jsonrpc":"2.0","id":1,"method":"ping"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: %d", rec.Code)
	}
}
