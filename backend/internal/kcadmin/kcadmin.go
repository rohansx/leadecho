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
// https://auth.example.com/realms/leadecho. adminHost (e.g.
// https://kc-admin.example.com), when set, serves the admin API instead of
// the issuer's host: public auth domains usually block /admin. Returns nil
// when OIDC isn't configured.
func New(issuer, adminHost, clientID, secret string) *Client {
	issuer = strings.TrimRight(issuer, "/")
	i := strings.LastIndex(issuer, "/realms/")
	if issuer == "" || clientID == "" || secret == "" || i < 0 {
		return nil
	}
	host := issuer[:i]
	if adminHost != "" {
		host = strings.TrimRight(adminHost, "/")
	}
	return &Client{
		tokenURL: issuer + "/protocol/openid-connect/token",
		adminURL: host + "/admin" + issuer[i:],
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

// CreateUser makes an enabled user with a permanent password and returns its
// Keycloak id. verified marks the email verified, so Keycloak won't ask for
// verification at sign-in. ErrUserExists: Keycloak already has that email.
// (Realms with "email as username" ignore any username we send, so callers
// keep the returned id to find this exact user again.)
func (c *Client) CreateUser(ctx context.Context, email, name, password string, verified bool) (string, error) {
	first, last, _ := strings.Cut(name, " ")
	res, err := c.do(ctx, http.MethodPost, "/users", map[string]any{
		"username": email, "email": email, "firstName": first, "lastName": last,
		"enabled": true, "emailVerified": verified, "requiredActions": []string{},
		"credentials": []map[string]any{{"type": "password", "value": password, "temporary": false}},
	})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusConflict:
		return "", ErrUserExists
	case res.StatusCode/100 != 2:
		return "", fmt.Errorf("keycloak create user: status %d", res.StatusCode)
	}
	// Location: <admin>/users/<id>
	loc := res.Header.Get("Location")
	id := loc[strings.LastIndex(loc, "/")+1:]
	if id == "" {
		return "", errors.New("keycloak create user: no id in Location")
	}
	return id, nil
}

// DeleteUser removes the user with this Keycloak id (gone already is fine).
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	res, err := c.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode/100 != 2 && res.StatusCode != http.StatusNotFound {
		return fmt.Errorf("keycloak delete user: status %d", res.StatusCode)
	}
	return nil
}

// DeleteUserRetry is DeleteUser with a few retries, detached from the
// request's cancellation: a temp login left behind keeps the operator's password alive.
func (c *Client) DeleteUserRetry(ctx context.Context, id string) error {
	ctx = context.WithoutCancel(ctx)
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = c.DeleteUser(ctx, id); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return err
}
