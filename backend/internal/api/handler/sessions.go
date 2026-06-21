package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/browser"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
)

var supportedSessionPlatforms = map[string]bool{
	"reddit":   true,
	"twitter":  true,
	"linkedin": true,
	"quora":    true,
}

// SessionResponse is returned for each configured platform session.
type SessionResponse struct {
	Platform         string  `json:"platform"`
	Username         *string `json:"username"`
	IsConfigured     bool    `json:"is_configured"`
	IsPinchtabOnline bool    `json:"is_pinchtab_online"`
}

// SessionHandler manages browser session cookies for Reddit and Twitter.
type SessionHandler struct {
	q        *database.Queries
	encKey   []byte
	pinchtab *browser.PinchtabClient
}

func NewSessionHandler(q *database.Queries, encKey []byte, pinchtab *browser.PinchtabClient) *SessionHandler {
	return &SessionHandler{q: q, encKey: encKey, pinchtab: pinchtab}
}

// List returns session status for all supported platforms.
func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(r.Context()) == nil
	}

	accounts, _ := h.q.ListPlatformSessions(r.Context(), wsID)
	configured := map[string]*database.PlatformAccount{}
	for i := range accounts {
		a := &accounts[i]
		configured[string(a.Platform)] = a
	}

	result := make([]SessionResponse, 0, len(supportedSessionPlatforms))
	for platform := range supportedSessionPlatforms {
		resp := SessionResponse{
			Platform:         platform,
			IsPinchtabOnline: pinchtabOnline,
		}
		if acc, ok := configured[platform]; ok && acc.AccessTokenEnc.Valid && acc.AccessTokenEnc.String != "" {
			resp.IsConfigured = true
			if acc.Username.Valid && acc.Username.String != "" {
				u := acc.Username.String
				resp.Username = &u
			}
		}
		result = append(result, resp)
	}

	writeJSON(w, http.StatusOK, result)
}

// Save stores an encrypted session cookie for a platform.
func (h *SessionHandler) Save(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	var body struct {
		SessionCookie string `json:"session_cookie"`
		Username      string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.SessionCookie == "" {
		writeError(w, http.StatusBadRequest, "session_cookie is required")
		return
	}

	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)
	claims := middleware.ClaimsFromContext(ctx)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	encrypted, err := crypto.Encrypt(h.encKey, body.SessionCookie)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt session")
		return
	}

	meta, _ := json.Marshal(map[string]string{"username": body.Username})

	_, err = h.q.UpsertPlatformSession(ctx, database.UpsertPlatformSessionParams{
		WorkspaceID:    wsID,
		UserID:         userID,
		Platform:       platform,
		Username:       pgtype.Text{String: body.Username, Valid: body.Username != ""},
		AccessTokenEnc: pgtype.Text{String: encrypted, Valid: true},
		Metadata:       meta,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(ctx) == nil
	}

	var usernamePtr *string
	if body.Username != "" {
		u := body.Username
		usernamePtr = &u
	}
	writeJSON(w, http.StatusOK, SessionResponse{
		Platform:         platform,
		Username:         usernamePtr,
		IsConfigured:     true,
		IsPinchtabOnline: pinchtabOnline,
	})
}

// Delete removes a platform session.
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	ctx := r.Context()
	wsID := middleware.WorkspaceID(ctx)

	if err := h.q.DeletePlatformSession(ctx, database.DeletePlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    platform,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	pinchtabOnline := false
	if h.pinchtab != nil {
		pinchtabOnline = h.pinchtab.Heartbeat(ctx) == nil
	}

	writeJSON(w, http.StatusOK, SessionResponse{
		Platform:         platform,
		IsConfigured:     false,
		IsPinchtabOnline: pinchtabOnline,
	})
}

// Test checks Pinchtab connectivity AND validates the stored session cookie
// by actually navigating to the platform and checking if the user is logged in.
func (h *SessionHandler) Test(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	if !supportedSessionPlatforms[platform] {
		writeError(w, http.StatusBadRequest, "unsupported platform")
		return
	}

	if h.pinchtab == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"pinchtab_online": false,
			"cookie_valid":    false,
			"message":         "Pinchtab not configured — set PINCHTAB_TOKEN to enable",
		})
		return
	}

	if err := h.pinchtab.Heartbeat(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"pinchtab_online": false,
			"cookie_valid":    false,
			"message":         err.Error(),
		})
		return
	}

	// Pinchtab is online — now validate the actual cookie.
	wsID := middleware.WorkspaceID(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	valid, msg := h.validateSessionCookie(ctx, wsID, platform)

	writeJSON(w, http.StatusOK, map[string]any{
		"pinchtab_online": true,
		"cookie_valid":    valid,
		"message":         msg,
	})
}

// validateSessionCookie navigates to the platform with the stored cookies
// and checks whether the session is still authenticated.
func (h *SessionHandler) validateSessionCookie(ctx context.Context, wsID, platform string) (bool, string) {
	session, err := h.q.GetPlatformSession(ctx, database.GetPlatformSessionParams{
		WorkspaceID: wsID,
		Platform:    platform,
	})
	if err != nil || !session.AccessTokenEnc.Valid || session.AccessTokenEnc.String == "" {
		return false, "No session cookie configured"
	}

	cookieStr, err := crypto.Decrypt(h.encKey, session.AccessTokenEnc.String)
	if err != nil {
		return false, "Failed to decrypt session cookie"
	}

	// Platform-specific config: domain, URL to navigate, and a JS check
	// that returns "logged_in" or "logged_out" based on page content.
	type platformCheck struct {
		domain    string
		navigate  string
		jsCheck   string
		cookieDom string
	}

	checks := map[string]platformCheck{
		"reddit": {
			domain:    "https://www.reddit.com",
			navigate:  "https://www.reddit.com",
			cookieDom: ".reddit.com",
			// Reddit: check for login link/button (logged out) vs user menu/create post (logged in)
			// Use textContent matching for i18n (Chinese Reddit shows "登录" for login)
			jsCheck: `(function() {
				var html = document.body ? document.body.innerHTML : '';
				var text = document.body ? document.body.innerText : '';
				// Logged out: login/signup links present
				if (text.match(/log ?in|sign ?in|登录|注册/i) && html.match(/login|signup|auth/i) && !text.match(/create.*post|创建.*帖子/i)) return 'logged_out';
				// Logged in: has create post / user menu / karma
				if (text.match(/create.*post|创建.*帖子|karma|carma/i) || html.match(/user-drawer|USER_DROPDOWN|header-user/i)) return 'logged_in';
				return 'unknown';
			})()`,
		},
		"twitter": {
			domain:    "https://x.com",
			navigate:  "https://x.com/home",
			cookieDom: ".x.com",
			jsCheck: `document.querySelector('[data-testid="loginButton"], a[href="/login"]') ? 'logged_out' : (document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"], [data-testid="AppTabBar_Profile_Link"]') ? 'logged_in' : 'unknown')`,
		},
		"linkedin": {
			domain:    "https://www.linkedin.com",
			navigate:  "https://www.linkedin.com/feed/",
			cookieDom: ".linkedin.com",
			jsCheck: `document.querySelector('.nav__button-secondary, a[href*="signin"]') ? 'logged_out' : (document.querySelector('.global-nav__me-photo, .feed-identity-module') ? 'logged_in' : 'unknown')`,
		},
		"quora": {
			domain:    "https://www.quora.com",
			navigate:  "https://www.quora.com",
			cookieDom: ".quora.com",
			// Quora geo-redirects to jp.quora.com / other locales; match by URL + multilingual text.
			// Logged-out: redirected to /login, or hero shows "Join Quora"/"Sign up"/"Log in".
			// Logged-in: has "Add Question" CTA (en/ja/zh) or a user avatar/profile menu.
			jsCheck: `(function() {
				var url = location.href;
				var text = document.body ? document.body.innerText : '';
				var html = document.body ? document.body.innerHTML : '';
				if (url.match(/\/login|\/signup/i)) return 'logged_out';
				if (text.match(/add question|ask question|質問を追加|問題を追加|添加问题|提问/i)) return 'logged_in';
				if (html.match(/class="[^"]*(?:avatar|user[_-]?menu|profile[_-]?photo|user[_-]?icon|q[_-]?menu)[^"]*"/i)) return 'logged_in';
				if (text.match(/^(join quora|sign up|log in|登録|新規登録|ログイン|登录|注册)/im)) return 'logged_out';
				if (html.match(/class="[^"]*(?:login[_-](?:button|cta)|signup[_-](?:button|cta)|register[_-]button)[^"]*"/i)) return 'logged_out';
				return 'unknown';
			})()`,
		},
	}

	check, ok := checks[platform]
	if !ok {
		return false, fmt.Sprintf("unsupported platform: %s", platform)
	}

	// Parse cookies
	cookies := parseCookieStringForHandler(cookieStr, check.cookieDom)

	// Navigate to domain first (IDPI requires being on the right domain)
	if err := h.pinchtab.Navigate(ctx, check.domain); err != nil {
		return false, fmt.Sprintf("Failed to navigate to %s: %v", platform, err)
	}

	// Inject cookies
	if err := h.pinchtab.InjectCookies(ctx, check.domain, cookies); err != nil {
		return false, fmt.Sprintf("Failed to inject cookies: %v", err)
	}

	// Navigate to the check page
	if err := h.pinchtab.Navigate(ctx, check.navigate); err != nil {
		return false, fmt.Sprintf("Failed to navigate: %v", err)
	}

	// Wait for page to render
	select {
	case <-ctx.Done():
		return false, "Context cancelled while waiting for page"
	case <-time.After(3 * time.Second):
	}

	// Run JS check
	result, err := h.pinchtab.EvaluateJS(ctx, check.jsCheck)
	if err != nil {
		return false, fmt.Sprintf("Failed to check login status: %v", err)
	}

	result = strings.TrimSpace(strings.Trim(result, `"`))

	switch result {
	case "logged_in":
		username := ""
		if session.Username.Valid && session.Username.String != "" {
			username = session.Username.String
		}
		if username != "" {
			return true, fmt.Sprintf("✓ Session valid — logged in as %s", username)
		}
		return true, "✓ Session valid — logged in"
	case "logged_out":
		return false, fmt.Sprintf("✗ Session expired — %s shows login page. Please update your cookies.", platform)
	default:
		// Couldn't determine — fetch diagnostics so the user can see what's on the page
		// (redirect target, anti-bot wall, locale, changed DOM, etc.).
		diagURL, _ := h.pinchtab.EvaluateJS(ctx, "location.href")
		diagURL = strings.TrimSpace(strings.Trim(diagURL, `"`))
		diagText, _ := h.pinchtab.GetText(ctx)
		diagText = strings.Join(strings.Fields(diagText), " ")
		if len(diagText) > 300 {
			diagText = diagText[:300]
		}
		return false, fmt.Sprintf("? Could not verify session for %s [url=%s] page: %s", platform, diagURL, diagText)
	}
}

// parseCookieStringForHandler parses a cookie header string into browser.Cookie structs.
func parseCookieStringForHandler(cookieStr, domain string) []browser.Cookie {
	var cookies []browser.Cookie
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if name == "" {
			continue
		}
		cookies = append(cookies, browser.Cookie{
			Name:   name,
			Value:  value,
			Domain: domain,
			Path:   "/",
		})
	}
	return cookies
}
