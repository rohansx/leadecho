package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PinchtabClient controls a Pinchtab browser sidecar via its HTTP/JSON API.
// Pinchtab runs a persistent Chrome instance with anti-bot fingerprint spoofing.
// If baseURL is empty or the sidecar is offline, callers should degrade gracefully.
type PinchtabClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// Cookie represents a browser cookie to inject into the Pinchtab session.
type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// New creates a PinchtabClient for the given base URL (e.g. "http://localhost:9867")
// and bearer token (BRIDGE_TOKEN env var on the sidecar).
func New(baseURL, token string) *PinchtabClient {
	return &PinchtabClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Heartbeat checks whether Pinchtab is reachable. Returns nil if online.
func (p *PinchtabClient) Heartbeat(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/heartbeat", nil)
	if err != nil {
		return err
	}
	p.setAuth(req)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("pinchtab unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pinchtab heartbeat: status %d", resp.StatusCode)
	}
	return nil
}

// Navigate navigates the browser to the given URL.
func (p *PinchtabClient) Navigate(ctx context.Context, url string) error {
	return p.post(ctx, "/navigate", map[string]string{"url": url}, nil)
}

// GetText returns the readable text content of the current page (~800 tokens/page).
func (p *PinchtabClient) GetText(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/text", nil)
	if err != nil {
		return "", err
	}
	p.setAuth(req)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pinchtab get text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pinchtab get text: status %d", resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Fall back to raw body if not JSON
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	}
	return result.Text, nil
}

// InjectCookies sets browser cookies for the current session.
// Inject before navigating to authenticated pages.
func (p *PinchtabClient) InjectCookies(ctx context.Context, cookies []Cookie) error {
	return p.post(ctx, "/cookies", cookies, nil)
}

// EvaluateJS executes JavaScript in the current page context and returns the string result.
func (p *PinchtabClient) EvaluateJS(ctx context.Context, script string) (string, error) {
	var result struct {
		Result string `json:"result"`
	}
	if err := p.post(ctx, "/evaluate", map[string]string{"expression": script}, &result); err != nil {
		return "", err
	}
	return result.Result, nil
}

// post sends a POST request with a JSON body. If out is non-nil, decodes the response into it.
func (p *PinchtabClient) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("pinchtab marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	p.setAuth(req)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("pinchtab %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pinchtab %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (p *PinchtabClient) setAuth(req *http.Request) {
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
}
