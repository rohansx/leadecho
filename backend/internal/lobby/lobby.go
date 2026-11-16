// Package lobby talks to Lobby (trylobby.app) claim links: an account built
// for a prospect is handed over once Lobby has verified their email.
// Server side only: the secret key never reaches a browser.
package lobby

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrInvalidCode: the claim code is invalid, already used or expired.
var ErrInvalidCode = errors.New("lobby: claim code invalid, used or expired")

type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// New returns nil when key is empty, so callers can treat "not configured" as nil.
func New(baseURL, key string) *Client {
	if key == "" {
		return nil
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Redemption is what a redeemed claim code tells us. Email is verified by Lobby.
type Redemption struct {
	AccountRef    string `json:"account_ref"`
	Email         string `json:"email"`
	Label         string `json:"label"`
	ClaimID       string `json:"claim_id"`
	SignupID      string `json:"signup_id"`
	ReporterToken string `json:"reporter_token"`
}

// Claim is a claim link for one account.
type Claim struct {
	ID       string `json:"id"`
	ClaimURL string `json:"claim_url"`
	State    string `json:"state"`
}

// Redeem spends a ?claim= code. Any non-200 answer is ErrInvalidCode: the
// caller shows one "link expired" page and never the reason.
func (c *Client) Redeem(ctx context.Context, code string) (Redemption, error) {
	var out Redemption
	status, err := c.post(ctx, "/v1/claims/redeem", map[string]string{"code": code}, &out)
	if err != nil {
		return out, err
	}
	if status != http.StatusOK || out.AccountRef == "" || out.Email == "" {
		return out, fmt.Errorf("%w (status %d)", ErrInvalidCode, status)
	}
	return out, nil
}

// CreateClaim opens a claim link for accountRef; Lobby revokes any older open
// link for the same ref. email, when set, locks the claim to that address.
func (c *Client) CreateClaim(ctx context.Context, accountRef, label, email string) (Claim, error) {
	body := map[string]string{"account_ref": accountRef}
	if label != "" {
		body["label"] = label
	}
	if email != "" {
		body["email"] = email
	}
	var out Claim
	status, err := c.post(ctx, "/v1/claims", body, &out)
	if err != nil {
		return out, err
	}
	if status/100 != 2 || out.ClaimURL == "" {
		return out, fmt.Errorf("lobby: create claim answered %d", status)
	}
	return out, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("lobby %s: %w", path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return res.StatusCode, fmt.Errorf("lobby %s: read: %w", path, err)
	}
	if res.StatusCode/100 == 2 {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, fmt.Errorf("lobby %s: decode: %w", path, err)
		}
	}
	return res.StatusCode, nil
}
