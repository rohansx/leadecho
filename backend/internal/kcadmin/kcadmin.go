// Package kcadmin manages users in the Keycloak realm LeadEcho signs in
// with, for operator-made temporary accounts. It authenticates as the OIDC
// client's service account (client_credentials), which needs the
// realm-management roles manage-users and view-users.
package kcadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUserExists: Keycloak already has a user with that email/username.
var ErrUserExists = errors.New("keycloak: user already exists")

type Client struct {
	tokenURL, adminURL string
	clientID, secret   string
	http               *http.Client
}

// New derives the admin API from an issuer like
// https://auth.example.com/realms/leadecho. Returns nil when OIDC isn't configured.
func New(issuer, clientID, secret string) *Client {
	issuer = strings.TrimRight(issuer, "/")
	i := strings.LastIndex(issuer, "/realms/")
	if issuer == "" || clientID == "" || secret == "" || i < 0 {
		return nil
	}
	return &Client{
		tokenURL: issuer + "/protocol/openid-connect/token",
		adminURL: issuer[:i] + "/admin" + issuer[i:],
		clientID: clientID, secret: secret,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// ponytail: a fresh service-account token per call; these are rare operator actions.
func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.clientID}, "client_secret": {c.secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak token: %w", err)
	}
	defer res.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token: status %d (is the client's service account enabled?)", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("keycloak token: bad response: %v", err)
	}
	return out.AccessToken, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.adminURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keycloak %s %s: %w", method, path, err)
	}
	return res, nil
}

// CreateUser makes an enabled user with a permanent password. verified marks
// the email as verified, so Keycloak won't ask for verification at sign-in.
func (c *Client) CreateUser(ctx context.Context, email, name, password string, verified bool) error {
	first, last, _ := strings.Cut(name, " ")
	res, err := c.do(ctx, http.MethodPost, "/users", map[string]any{
		"username": email, "email": email, "firstName": first, "lastName": last,
		"enabled": true, "emailVerified": verified, "requiredActions": []string{},
		"credentials": []map[string]any{{"type": "password", "value": password, "temporary": false}},
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusConflict:
		return ErrUserExists
	case res.StatusCode/100 != 2:
		return fmt.Errorf("keycloak create user: status %d", res.StatusCode)
	}
	return nil
}

// DeleteUserByEmail removes the user whose email is exactly email (no-op if none).
func (c *Client) DeleteUserByEmail(ctx context.Context, email string) error {
	res, err := c.do(ctx, http.MethodGet, "/users?exact=true&email="+url.QueryEscape(email), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("keycloak find user: status %d", res.StatusCode)
	}
	var users []struct{ ID, Email string }
	if err := json.NewDecoder(io.LimitReader(res.Body, 256<<10)).Decode(&users); err != nil {
		return fmt.Errorf("keycloak find user: %w", err)
	}
	for _, u := range users {
		if !strings.EqualFold(u.Email, email) {
			continue
		}
		del, err := c.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(u.ID), nil)
		if err != nil {
			return err
		}
		del.Body.Close()
		if del.StatusCode/100 != 2 && del.StatusCode != http.StatusNotFound {
			return fmt.Errorf("keycloak delete user: status %d", del.StatusCode)
		}
	}
	return nil
}
