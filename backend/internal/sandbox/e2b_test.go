package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeE2B plays the E2B control API, the envd file API and the tool server.
type fakeE2B struct {
	mu       sync.Mutex
	token    string
	killed   int
	created  int
	notReady int // tool calls answered 503 before the token is "read"
	srv      *httptest.Server
}

func (f *fakeE2B) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
		if r.Header.Get("X-API-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if net, _ := body["network"].(map[string]any); net["allowPublicTraffic"] != false {
			http.Error(w, "ports must be private", http.StatusBadRequest)
			return
		}
		f.created++
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sandboxID":"sb1","envdAccessToken":"envd","trafficAccessToken":"traffic","domain":null}`)
	case r.Method == http.MethodDelete && r.URL.Path == "/sandboxes/sb1":
		f.killed++
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/files":
		if r.Header.Get("X-Access-Token") != "envd" || r.URL.Query().Get("path") != "/home/user/.tool_token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.token = string(b)
		_, _ = io.WriteString(w, `[{"name":".tool_token"}]`)
	case r.URL.Path == "/browse":
		if r.Header.Get("e2b-traffic-access-token") != "traffic" || r.Header.Get("Authorization") != "Bearer "+f.token || f.token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.notReady > 0 {
			f.notReady--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"final_url":"https://jane.dev/","text":"hi"}`)
	default:
		http.NotFound(w, r)
	}
}

func newFake(t *testing.T) (*fakeE2B, *Client) {
	f := &fakeE2B{notReady: 1}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(f.srv.Close)
	c := New("k", "")
	c.base, c.http = f.srv.URL, f.srv.Client()
	c.portURL = func(int, string, string) string { return f.srv.URL }
	return f, c
}

func TestSessionStartsLazilyHandsOverTokenAndKills(t *testing.T) {
	f, c := newFake(t)
	s := c.Open()
	if f.created != 0 {
		t.Fatal("no sandbox before the first tool call")
	}
	out, err := s.Run(context.Background(), "browse", json.RawMessage(`{"url":"https://jane.dev"}`))
	if err != nil || !strings.Contains(out, "jane.dev") {
		t.Fatalf("run: %q %v", out, err)
	}
	if _, err := s.Run(context.Background(), "browse", json.RawMessage(`{"url":"https://jane.dev"}`)); err != nil || f.created != 1 {
		t.Fatalf("second call should reuse the sandbox: created=%d err=%v", f.created, err)
	}
	if len(f.token) != 48 {
		t.Fatalf("per-session token not handed over: %q", f.token)
	}
	s.Close()
	s.Close() // idempotent
	if f.killed != 1 {
		t.Fatalf("killed %d times, want 1", f.killed)
	}
}

func TestSessionRejectsUnknownToolsAndBadInput(t *testing.T) {
	f, c := newFake(t)
	s := c.Open()
	defer s.Close()
	if _, err := s.Run(context.Background(), "shell", json.RawMessage(`{}`)); err == nil {
		t.Error("only the three research tools may run")
	}
	if _, err := s.Run(context.Background(), "browse", json.RawMessage(`{`)); err == nil {
		t.Error("invalid JSON must be refused")
	}
	if f.created != 0 {
		t.Error("refused calls must not start a sandbox")
	}
}

func TestNilClientIsDisabled(t *testing.T) {
	if New("", "") != nil {
		t.Fatal("no key: disabled")
	}
	var c *Client
	if c.Open() != nil {
		t.Fatal("nil client opens nothing")
	}
	var s *Session
	s.Close() // must not panic
}
