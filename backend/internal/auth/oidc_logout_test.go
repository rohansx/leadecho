package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOIDCLogoutEndsIssuerSession(t *testing.T) {
	srv := oidcServer(t, `["create"]`)
	p := NewOIDCProvider(srv.URL+"/realms/x", "web", "s", "https://app/cb", nil)
	h := NewOAuthHandler(p, &EmailHandler{frontendURL: "https://app"})

	for _, tc := range []struct {
		name, hint string
	}{{"with id token", "tok123"}, {"without id token", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, idTokenPath, nil)
			if tc.hint != "" {
				req.AddCookie(&http.Cookie{Name: idTokenCookie, Value: tc.hint})
			}
			rec := httptest.NewRecorder()
			h.Logout(rec, req)

			u, err := url.Parse(rec.Header().Get("Location"))
			if err != nil || u.Scheme+"://"+u.Host+u.Path != srv.URL+"/logout" {
				t.Fatalf("redirect = %q, want issuer end_session", rec.Header().Get("Location"))
			}
			q := u.Query()
			if q.Get("client_id") != "web" || q.Get("post_logout_redirect_uri") != "https://app/login" || q.Get("id_token_hint") != tc.hint {
				t.Fatalf("query = %v", q)
			}
			if c := rec.Result().Cookies(); len(c) != 1 || c[0].Name != idTokenCookie || c[0].MaxAge >= 0 {
				t.Fatalf("id token cookie not cleared: %v", c)
			}
		})
	}
}
