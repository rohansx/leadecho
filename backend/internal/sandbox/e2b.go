// Package sandbox runs the identity research agent's tools (browser,
// username sweep, YouTube) in an E2B sandbox built from identity-engine/.
// A session starts its sandbox on the first tool call and kills it on Close,
// so research that never needs a browser never pays for one.
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	apiBase      = "https://api.e2b.app"
	toolPort     = 8000
	envdPort     = 49983
	sandboxLife  = 10 * 60 // seconds; Close normally ends it sooner
	maxToolReply = 256 << 10
)

// Tools the template's server exposes.
var tools = map[string]string{"browse": "/browse", "username_sweep": "/username_sweep", "youtube": "/youtube"}

// Client creates sessions. A nil *Client is valid and disabled.
type Client struct {
	apiKey, template, base string
	http                   *http.Client
	// portURL is the public URL of a sandbox port (swapped in tests).
	portURL func(port int, id, domain string) string
}

func defaultPortURL(port int, id, domain string) string {
	return fmt.Sprintf("https://%d-%s.%s", port, id, domain)
}

// New returns nil when apiKey is empty (feature off).
func New(apiKey, template string) *Client {
	if apiKey == "" {
		return nil
	}
	if template == "" {
		template = "leadecho-identity"
	}
	return &Client{apiKey: apiKey, template: template, base: apiBase, http: &http.Client{Timeout: 150 * time.Second}, portURL: defaultPortURL}
}

// Session is one research run's sandbox. Safe for sequential tool calls.
type Session struct {
	c  *Client
	mu sync.Mutex

	id, domain, traffic, token string
	started                    bool
	startErr                   error
}

// Open returns a session; the sandbox starts on first use.
func (c *Client) Open() *Session {
	if c == nil {
		return nil
	}
	return &Session{c: c}
}

type created struct {
	SandboxID          string  `json:"sandboxID"`
	EnvdAccessToken    string  `json:"envdAccessToken"`
	TrafficAccessToken string  `json:"trafficAccessToken"`
	Domain             *string `json:"domain"`
}

func (s *Session) start(ctx context.Context) error {
	if s.started {
		return s.startErr
	}
	s.started = true
	body, _ := json.Marshal(map[string]any{
		"templateID": s.c.template, "timeout": sandboxLife,
		"network": map[string]any{"allowPublicTraffic": false}, // ports need the traffic token
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.c.base+"/sandboxes", bytes.NewReader(body))
	if err != nil {
		s.startErr = err
		return err
	}
	req.Header.Set("X-API-Key", s.c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	var cr created
	if err := s.c.do(req, http.StatusCreated, &cr); err != nil {
		s.startErr = fmt.Errorf("sandbox create: %w", err)
		return s.startErr
	}
	s.id, s.traffic, s.domain = cr.SandboxID, cr.TrafficAccessToken, "e2b.app"
	if cr.Domain != nil && *cr.Domain != "" {
		s.domain = *cr.Domain
	}
	// The tool server was started at build time, so it can't see env vars set
	// now: hand it a fresh per-session token through the sandbox's file API.
	tok := make([]byte, 24)
	if _, err := rand.Read(tok); err != nil {
		s.startErr = fmt.Errorf("sandbox token: %w", err)
		s.kill()
		return s.startErr
	}
	s.token = hex.EncodeToString(tok)
	if err := s.writeToken(ctx, cr.EnvdAccessToken); err != nil {
		s.startErr = fmt.Errorf("sandbox token: %w", err)
		s.kill()
		return s.startErr
	}
	return nil
}

func (s *Session) writeToken(ctx context.Context, envdToken string) error {
	u := s.c.portURL(envdPort, s.id, s.domain) + "/files?path=/home/user/.tool_token&username=user"
	var last error
	for attempt := 0; attempt < 10; attempt++ { // envd takes a few seconds to come up
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte(s.token)))
		if err != nil {
			return err
		}
		req.Header.Set("X-Access-Token", envdToken)
		req.Header.Set("Content-Type", "application/octet-stream")
		if last = s.c.do(req, http.StatusOK, nil); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return last
}

// Run calls one tool. The server answers 503 until it has read the token,
// which can take a moment after it is written.
func (s *Session) Run(ctx context.Context, tool string, input json.RawMessage) (string, error) {
	path, ok := tools[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", tool)
	}
	if !json.Valid(input) {
		return "", errors.New("tool input is not valid JSON")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.start(ctx); err != nil {
		return "", err
	}
	u := s.c.portURL(toolPort, s.id, s.domain) + path
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(input))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+s.token)
		req.Header.Set("e2b-traffic-access-token", s.traffic)
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.c.http.Do(req)
		if err != nil {
			return "", fmt.Errorf("sandbox %s: %w", tool, err)
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxToolReply))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusServiceUnavailable && attempt < 5:
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		case resp.StatusCode >= 400:
			return "", fmt.Errorf("sandbox %s: status %d: %s", tool, resp.StatusCode, clip(string(b), 200))
		}
		return string(b), nil
	}
}

// Close kills the sandbox (if it was started).
func (s *Session) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != "" {
		s.kill()
	}
}

func (s *Session) kill() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.c.base+"/sandboxes/"+s.id, nil)
	if err != nil {
		return
	}
	req.Header.Set("X-API-Key", s.c.apiKey)
	_ = s.c.do(req, http.StatusNoContent, nil) // the sandbox also expires on its own
	s.id = ""
}

func (c *Client) do(req *http.Request, want int, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != want {
		return fmt.Errorf("status %d: %s", resp.StatusCode, clip(string(b), 200))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
