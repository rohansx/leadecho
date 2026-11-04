package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// RedditOAuth is Reddit's app-only (client_credentials) OAuth. Reddit blocks
// unauthenticated JSON from most server IPs (403), so hosted deployments
// register a free "script"/"web" app and set REDDIT_CLIENT_ID/SECRET.
type RedditOAuth struct {
	clientID, secret, userAgent string
	tokenBase, apiBase          string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewRedditOAuth(clientID, secret, userAgent, tokenBase, apiBase string) *RedditOAuth {
	return &RedditOAuth{clientID: clientID, secret: secret, userAgent: userAgent,
		tokenBase: strings.TrimRight(tokenBase, "/"), apiBase: strings.TrimRight(apiBase, "/")}
}

// RedditOAuthFromEnv returns nil (public JSON path) unless both credentials are set.
func RedditOAuthFromEnv() *RedditOAuth {
	id, secret := os.Getenv("REDDIT_CLIENT_ID"), os.Getenv("REDDIT_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil
	}
	ua := os.Getenv("REDDIT_USER_AGENT")
	if ua == "" {
		ua = "web:leadecho:1.0" // Reddit asks for <platform>:<app id>:<version> (by /u/<owner>)
	}
	return NewRedditOAuth(id, secret, ua, "https://www.reddit.com", "https://oauth.reddit.com")
}

func (o *RedditOAuth) bearer(ctx context.Context, c *http.Client, refresh bool) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !refresh && o.token != "" && time.Now().Before(o.expires) {
		return o.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenBase+"/api/v1/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(o.clientID, o.secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", o.userAgent)
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("reddit token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &StatusError{Code: resp.StatusCode}
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("reddit token: bad response")
	}
	o.token = tok.AccessToken
	o.expires = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return o.token, nil
}

// getJSON fetches pathAndQuery from oauth.reddit.com, refreshing the token once on 401.
func (o *RedditOAuth) getJSON(ctx context.Context, c *http.Client, pathAndQuery string, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := o.bearer(ctx, c, attempt > 0)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.apiBase+pathAndQuery, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", o.userAgent)
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return &StatusError{Code: resp.StatusCode}
		}
		return json.NewDecoder(io.LimitReader(resp.Body, maxSourceBody)).Decode(out)
	}
	return &StatusError{Code: http.StatusUnauthorized}
}
