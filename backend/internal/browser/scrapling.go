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

// ScraplingClient controls a Scrapling browser sidecar via its HTTP/JSON API.
// Scrapling provides stealth browsing with anti-bot bypass and adaptive DOM parsing.
// Used to read websites (onboarding, thread context); no session cookies.
type ScraplingClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewScrapling creates a ScraplingClient for the given base URL (e.g. "http://localhost:9869")
// and bearer token (SCRAPLING_TOKEN env var on the sidecar).
func NewScrapling(baseURL, token string) *ScraplingClient {
	return &ScraplingClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Heartbeat checks whether the Scrapling sidecar is reachable. Returns nil if online.
func (c *ScraplingClient) Heartbeat(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("scrapling unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scrapling heartbeat: status %d", resp.StatusCode)
	}
	return nil
}

// Navigate navigates the stealth browser to the given URL.
func (c *ScraplingClient) Navigate(ctx context.Context, url string) error {
	return c.post(ctx, "/navigate", map[string]string{"url": url}, nil)
}

// GetText returns the readable text content of the current page.
func (c *ScraplingClient) GetText(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/text", nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("scrapling get text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scrapling get text: status %d", resp.StatusCode)
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

func (c *ScraplingClient) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("scrapling marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("scrapling %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scrapling %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *ScraplingClient) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
