package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLogtoProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oidc/me" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"sub":"u1","email":"a@b.co","email_verified":true,"username":"ann"}`))
	}))
	defer srv.Close()

	p := NewLogtoProvider(srv.URL+"/", "app", "secret", "https://x/cb")
	if p.oauth.Endpoint.AuthURL != srv.URL+"/oidc/auth" || p.oauth.Endpoint.TokenURL != srv.URL+"/oidc/token" {
		t.Fatalf("endpoints: %+v", p.oauth.Endpoint)
	}
	id, err := p.fetch(context.Background(), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if id != (identity{ID: "u1", Email: "a@b.co", EmailVerified: true, Name: "ann"}) {
		t.Fatalf("identity: %+v", id)
	}

	authURL := func(target string) url.Values {
		rec := httptest.NewRecorder()
		NewOAuthHandler(p, &EmailHandler{}).Login(rec, httptest.NewRequest(http.MethodGet, target, nil))
		u, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		return u.Query()
	}
	if got := authURL("/auth/logto?screen=register").Get("first_screen"); got != "register" {
		t.Fatalf("register screen: %q", got)
	}
	if got := authURL("/auth/logto").Get("first_screen"); got != "" {
		t.Fatalf("login screen: %q", got)
	}
}
