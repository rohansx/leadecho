package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const resendEndpoint = "https://api.resend.com/emails"

// Resend sends one email through the Resend API. With no API key it is
// disabled and Send is a no-op (callers decide what that means). Endpoint
// overrides the API URL (tests).
type Resend struct {
	APIKey   string
	From     string
	Endpoint string
}

func (r Resend) Enabled() bool { return r.APIKey != "" }

func (r Resend) Send(ctx context.Context, to []string, subject, html string) error {
	if !r.Enabled() {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"from": r.From, "to": to, "subject": subject, "html": html,
	})
	if err != nil {
		return err
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = resendEndpoint
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
