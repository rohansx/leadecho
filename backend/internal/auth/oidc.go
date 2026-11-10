package auth

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// NewOIDCProvider signs in through any OpenID Connect issuer (Keycloak realm,
// Logto, ...). Endpoints come from the issuer's discovery document, fetched on
// first use and retried until it succeeds, so the API boots even if the
// identity server is down.
//
// issuer examples: https://auth.leadecho.app/realms/leadecho, https://auth.example.com/oidc
//
// idps lists the Keycloak identity providers (e.g. "google", "github") that
// ?idp=<alias> may jump straight to via kc_idp_hint; anything else is ignored.
func NewOIDCProvider(issuer, clientID, secret, redirectURL string, idps []string) *Provider {
	p := &Provider{Name: "oidc", oauth: &oauth2.Config{
		ClientID: clientID, ClientSecret: secret, RedirectURL: redirectURL,
		Scopes: []string{"openid", "email", "profile"},
	}}
	d := &oidcDiscovery{issuer: strings.TrimRight(issuer, "/"), client: &http.Client{Timeout: 10 * time.Second}}
	p.prepare = func(ctx context.Context) error { return d.load(ctx, p) }
	p.fetch = func(ctx context.Context, c *http.Client) (identity, error) { return fetchOIDCUser(ctx, c, p.apiURL) }
	p.authOpts = func(r *http.Request) []oauth2.AuthCodeOption {
		opts := d.registerOpts(r)
		if idp := r.URL.Query().Get("idp"); idp != "" && slices.Contains(idps, idp) {
			opts = append(opts, oauth2.SetAuthURLParam("kc_idp_hint", idp))
		}
		return opts
	}
	return p
}

type oidcDiscovery struct {
	issuer string
	client *http.Client

	mu     sync.Mutex
	done   bool
	prompt bool // issuer supports prompt=create (OIDC "initiate user registration")
}

func (d *oidcDiscovery) load(ctx context.Context, p *Provider) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done {
		return nil
	}
	var doc struct {
		Issuer        string   `json:"issuer"`
		Authorization string   `json:"authorization_endpoint"`
		Token         string   `json:"token_endpoint"`
		UserInfo      string   `json:"userinfo_endpoint"`
		PromptValues  []string `json:"prompt_values_supported"`
	}
	if err := getJSON(ctx, d.client, d.issuer+"/.well-known/openid-configuration", &doc); err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	if strings.TrimRight(doc.Issuer, "/") != d.issuer {
		return fmt.Errorf("oidc discovery: issuer %q does not match configured %q", doc.Issuer, d.issuer)
	}
	if doc.Authorization == "" || doc.Token == "" || doc.UserInfo == "" {
		return fmt.Errorf("oidc discovery: missing endpoints")
	}
	p.oauth.Endpoint = oauth2.Endpoint{AuthURL: doc.Authorization, TokenURL: doc.Token}
	p.apiURL = doc.UserInfo
	d.prompt = slices.Contains(doc.PromptValues, "create")
	d.done = true
	return nil
}

// registerOpts opens the issuer's sign-up screen for ?screen=register:
// prompt=create where supported (Keycloak), else Logto's first_screen.
func (d *oidcDiscovery) registerOpts(r *http.Request) []oauth2.AuthCodeOption {
	if r.URL.Query().Get("screen") != "register" {
		return nil
	}
	d.mu.Lock()
	prompt := d.prompt
	d.mu.Unlock()
	if prompt {
		return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("prompt", "create")}
	}
	return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("first_screen", "register")}
}

func fetchOIDCUser(ctx context.Context, c *http.Client, userinfoURL string) (identity, error) {
	var u struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Username          string `json:"username"`
		Picture           string `json:"picture"`
	}
	if err := getJSON(ctx, c, userinfoURL, &u); err != nil {
		return identity{}, err
	}
	name := u.Name
	for _, alt := range []string{u.PreferredUsername, u.Username} {
		if name == "" {
			name = alt
		}
	}
	return identity{ID: u.Sub, Email: u.Email, EmailVerified: u.EmailVerified, Name: name, Avatar: u.Picture}, nil
}
