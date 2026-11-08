package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func oidcServer(t *testing.T, prompts string) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/x/.well-known/openid-configuration":
			w.Write([]byte(`{"issuer":"` + srv.URL + `/realms/x","authorization_endpoint":"` + srv.URL + `/auth","token_endpoint":"` + srv.URL + `/token","userinfo_endpoint":"` + srv.URL + `/me","prompt_values_supported":` + prompts + `}`))
		case "/me":
			w.Write([]byte(`{"sub":"u1","email":"a@b.co","email_verified":true,"preferred_username":"ann"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func authQuery(t *testing.T, p *Provider, target string) url.Values {
	t.Helper()
	rec := httptest.NewRecorder()
	NewOAuthHandler(p, &EmailHandler{}).Login(rec, httptest.NewRequest(http.MethodGet, target, nil))
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestOIDCProvider(t *testing.T) {
	srv := oidcServer(t, `["none","login","create"]`)
	p := NewOIDCProvider(srv.URL+"/realms/x/", "app", "secret", "https://x/cb")

	q := authQuery(t, p, "/auth/oidc?screen=register")
	if q.Get("prompt") != "create" || q.Get("client_id") != "app" {
		t.Fatalf("register redirect: %v", q)
	}
	if p.oauth.Endpoint.TokenURL != srv.URL+"/token" {
		t.Fatalf("token url: %q", p.oauth.Endpoint.TokenURL)
	}
	if q := authQuery(t, p, "/auth/oidc"); q.Get("prompt") != "" || q.Get("first_screen") != "" {
		t.Fatalf("login redirect: %v", q)
	}

	id, err := p.fetch(context.Background(), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if id != (identity{ID: "u1", Email: "a@b.co", EmailVerified: true, Name: "ann"}) {
		t.Fatalf("identity: %+v", id)
	}
}

func TestOIDCProviderWithoutPromptCreate(t *testing.T) {
	srv := oidcServer(t, `["consent"]`)
	p := NewOIDCProvider(srv.URL+"/realms/x", "app", "secret", "https://x/cb")
	if got := authQuery(t, p, "/auth/oidc?screen=register").Get("first_screen"); got != "register" {
		t.Fatalf("first_screen: %q", got)
	}
}

func TestOIDCProviderRejectsIssuerMismatch(t *testing.T) {
	srv := oidcServer(t, `[]`)
	p := NewOIDCProvider(srv.URL+"/realms/other", "app", "secret", "https://x/cb")
	if err := p.prepare(context.Background()); err == nil {
		t.Fatal("want discovery error for unknown issuer path")
	}
}
