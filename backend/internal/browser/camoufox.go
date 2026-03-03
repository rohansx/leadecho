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

// CamoufoxClient controls a Camoufox browser sidecar via its HTTP/JSON API.
// Camoufox runs a persistent stealth Firefox instance with human-mimicry enabled.
// The API contract is identical to PinchtabClient; use this for Pro-tier LinkedIn crawls.
type CamoufoxClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewCamoufox creates a CamoufoxClient for the given base URL (e.g. "http://localhost:9868")
// and bearer token (CAMOUFOX_TOKEN env var on the sidecar).
func NewCamoufox(baseURL, token string) *CamoufoxClient {
	return &CamoufoxClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Heartbeat checks whether Camoufox is reachable. Returns nil if online.
func (c *CamoufoxClient) Heartbeat(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("camoufox unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("camoufox heartbeat: status %d", resp.StatusCode)
	}
	return nil
}

// Navigate navigates the browser to the given URL.
func (c *CamoufoxClient) Navigate(ctx context.Context, url string) error {
	return c.post(ctx, "/navigate", map[string]string{"url": url}, nil)
}

// GetText returns the readable text content of the current page.
func (c *CamoufoxClient) GetText(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/text", nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("camoufox get text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("camoufox get text: status %d", resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	}
	return result.Text, nil
}

// InjectCookies sets browser cookies for the current session.
func (c *CamoufoxClient) InjectCookies(ctx context.Context, cookies []Cookie) error {
	return c.post(ctx, "/cookies", cookies, nil)
}

// EvaluateJS executes JavaScript in the current page context and returns the string result.
func (c *CamoufoxClient) EvaluateJS(ctx context.Context, script string) (string, error) {
	var result struct {
		Result string `json:"result"`
	}
	if err := c.post(ctx, "/evaluate", map[string]string{"expression": script}, &result); err != nil {
		return "", err
	}
	return result.Result, nil
}

func (c *CamoufoxClient) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("camoufox marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("camoufox %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("camoufox %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *CamoufoxClient) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
