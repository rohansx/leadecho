package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"leadecho/internal/database"
)

var errAccountExists = errors.New("an unproven password account already uses this email")

// identity is what a provider tells us about the signed-in person.
type identity struct {
	ID            string
	Email         string
	EmailVerified bool
	Name          string
	Avatar        string
}

// Provider is one OAuth sign-in option (Google, GitHub).
type Provider struct {
	Name   string // "google" | "github"; also the external-id prefix
	oauth  *oauth2.Config
	fetch  func(ctx context.Context, c *http.Client) (identity, error)
	apiURL string // overridable in tests
}

func NewGoogleProvider(clientID, secret, redirectURL string) *Provider {
	p := &Provider{Name: "google", apiURL: "https://www.googleapis.com", oauth: &oauth2.Config{
		ClientID: clientID, ClientSecret: secret, RedirectURL: redirectURL,
		Scopes: []string{"openid", "email", "profile"}, Endpoint: google.Endpoint,
	}}
	p.fetch = p.fetchGoogle
	return p
}

func NewGitHubProvider(clientID, secret, redirectURL string) *Provider {
	p := &Provider{Name: "github", apiURL: "https://api.github.com", oauth: &oauth2.Config{
		ClientID: clientID, ClientSecret: secret, RedirectURL: redirectURL,
		Scopes: []string{"read:user", "user:email"}, Endpoint: github.Endpoint,
	}}
	p.fetch = p.fetchGitHub
	return p
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) fetchGoogle(ctx context.Context, c *http.Client) (identity, error) {
	var u struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := getJSON(ctx, c, p.apiURL+"/oauth2/v2/userinfo", &u); err != nil {
		return identity{}, err
	}
	return identity{ID: u.ID, Email: u.Email, EmailVerified: u.VerifiedEmail, Name: u.Name, Avatar: u.Picture}, nil
}

func (p *Provider) fetchGitHub(ctx context.Context, c *http.Client) (identity, error) {
	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := getJSON(ctx, c, p.apiURL+"/user", &u); err != nil {
		return identity{}, err
	}
	var emails []githubEmail
	if err := getJSON(ctx, c, p.apiURL+"/user/emails", &emails); err != nil {
		return identity{}, err
	}
	email := primaryVerifiedEmail(emails)
	name := u.Name
	if name == "" {
		name = u.Login
	}
	return identity{ID: strconv.FormatInt(u.ID, 10), Email: email, EmailVerified: email != "", Name: name, Avatar: u.AvatarURL}, nil
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// primaryVerifiedEmail returns the primary email only if GitHub has verified it.
func primaryVerifiedEmail(emails []githubEmail) string {
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	return ""
}

// OAuthHandler runs the redirect + callback for a provider.
type OAuthHandler struct {
	p    *Provider
	core *EmailHandler
}

func NewOAuthHandler(p *Provider, core *EmailHandler) *OAuthHandler {
	return &OAuthHandler{p: p, core: core}
}

func (h *OAuthHandler) stateCookie() string { return "oauth_state_" + h.p.Name }

// Login redirects to the provider's consent screen.
func (h *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	http.SetCookie(w, &http.Cookie{
		Name: h.stateCookie(), Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: h.core.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.p.oauth.AuthCodeURL(state), http.StatusFound)
}

// Callback verifies state, exchanges the code, then signs in / links / creates the user.
func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func(code string) {
		http.Redirect(w, r, h.core.frontendURL+"/login?error="+code, http.StatusFound)
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		fail("oauth_denied")
		return
	}
	sc, err := r.Cookie(h.stateCookie())
	if err != nil || sc.Value == "" || sc.Value != q.Get("state") {
		fail("invalid_state")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: h.stateCookie(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.core.secure})

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := h.p.oauth.Exchange(ctx, q.Get("code"))
	if err != nil {
		h.core.logger.Warn().Err(err).Str("provider", h.p.Name).Msg("oauth code exchange failed")
		fail("auth_failed")
		return
	}
	id, err := h.p.fetch(ctx, h.p.oauth.Client(ctx, tok))
	if err != nil {
		h.core.logger.Error().Err(err).Str("provider", h.p.Name).Msg("oauth fetch identity")
		fail("auth_failed")
		return
	}
	if id.ID == "" || id.Email == "" || !id.EmailVerified {
		fail("email_unverified_provider")
		return
	}

	user, wsID, created, err := h.signIn(ctx, id)
	if errors.Is(err, errAccountExists) {
		fail("account_exists")
		return
	}
	if err != nil {
		h.core.logger.Error().Err(err).Str("provider", h.p.Name).Msg("oauth sign-in")
		fail("auth_failed")
		return
	}
	st, err := h.core.queries.GetUserAuthState(ctx, user.ID)
	if err != nil || !h.core.startSession(w, user, wsID, st.SessionVersion) {
		fail("auth_failed")
		return
	}
	if created {
		go h.core.mail.welcome(context.Background(), user.Name, user.Email)
	}
	http.Redirect(w, r, h.core.frontendURL+"/inbox", http.StatusFound)
}

// signIn: external id → existing account by (provider-verified) email → new account.
func (h *OAuthHandler) signIn(ctx context.Context, id identity) (database.User, string, bool, error) {
	q := h.core.queries
	ext := h.p.Name + "_" + id.ID
	users, err := q.ListUsersByExternalID(ctx, ext)
	if err != nil {
		return database.User{}, "", false, err
	}
	if len(users) > 0 {
		return users[0], users[0].WorkspaceID, false, nil
	}

	email := normalizeEmail(id.Email)
	existing, err := q.FindUserByEmail(ctx, email)
	switch {
	case err == nil:
		// The provider verified this email; whether we may link depends on
		// whether the local account's ownership of it was ever proven.
		st, err := q.GetUserAuthState(ctx, existing.ID)
		if err != nil {
			return database.User{}, "", false, err
		}
		switch {
		case st.EmailVerified:
			// Proven (verify link, reset, or another OAuth provider): link.
		case !existing.PasswordHash.Valid:
			if err := q.SetEmailVerified(ctx, existing.ID); err != nil {
				return database.User{}, "", false, err
			}
		case h.core.requireVerif:
			// Unproven password account: its password may have been set by an
			// attacker who pre-registered this email. Drop it and revoke any
			// sessions before the real owner signs in.
			if err := q.ClearPasswordAndRevoke(ctx, existing.ID); err != nil {
				return database.User{}, "", false, err
			}
			h.core.versions.Forget(existing.ID)
		default:
			// Verification is off, so no password account is ever proven.
			// Linking would hand a pre-registered account to whoever signs in
			// with OAuth (or vice versa); make them use the password instead.
			return database.User{}, "", false, errAccountExists
		}
		return existing, existing.WorkspaceID, false, nil
	case !isNotFound(err):
		return database.User{}, "", false, err
	}

	name := strings.TrimSpace(id.Name)
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	user, ws, err := h.core.createAccount(ctx, ext, email, name, id.Avatar, "", true)
	if err != nil {
		return database.User{}, "", false, err
	}
	return user, ws.ID, true, nil
}
