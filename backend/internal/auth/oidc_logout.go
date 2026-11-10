package auth

import (
	"net/http"
	"net/url"
)

// idTokenCookie holds the OIDC id_token so logout can pass it as id_token_hint
// (Keycloak then ends its session without a "Do you want to log out?" screen).
// Scoped to the logout route and SameSite=Strict: a cross-site link can't use it
// to sign someone out silently.
const idTokenCookie = "oidc_idt"

const idTokenPath = "/api/v1/auth/oidc/logout"

func (h *OAuthHandler) setIDToken(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: idTokenCookie, Value: token, Path: idTokenPath, MaxAge: maxAge,
		HttpOnly: true, Secure: h.core.secure, SameSite: http.SameSiteStrictMode,
	})
}

// Logout ends the identity provider's session too. Clearing only our cookie is
// not enough: the issuer's SSO session would sign the user straight back in on
// the next /login. Call after POST /auth/logout has cleared the app session.
func (h *OAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	h.setIDToken(w, "", -1)
	if h.p.endSession == "" {
		http.Redirect(w, r, h.core.frontendURL+"/", http.StatusFound)
		return
	}
	http.Redirect(w, r, h.logoutURL(r), http.StatusFound)
}

func (h *OAuthHandler) logoutURL(r *http.Request) string {
	q := url.Values{
		"client_id":                {h.p.oauth.ClientID},
		"post_logout_redirect_uri": {h.core.frontendURL + "/login"},
	}
	if c, err := r.Cookie(idTokenCookie); err == nil && c.Value != "" {
		q.Set("id_token_hint", c.Value)
	}
	return h.p.endSession + "?" + q.Encode()
}
