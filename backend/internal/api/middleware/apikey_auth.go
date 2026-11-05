package middleware

// mcp: workspace API key auth for the MCP server and the public REST API.

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/apikey"
	"leadecho/internal/auth"
	"leadecho/internal/database"
)

// APIKeyStore is the slice of database.Queries the middleware needs.
type APIKeyStore interface {
	GetActiveWorkspaceAPIKeyByHash(ctx context.Context, keyHash string) (database.WorkspaceApiKey, error)
	TouchWorkspaceAPIKey(ctx context.Context, arg database.TouchWorkspaceAPIKeyParams) error
}

type apiKeyScopesKey struct{}

const apiKeyRateWindow = time.Minute

// APIKeyAuth authenticates "Authorization: Bearer le_live_…" and injects the
// same claims the JWT middleware does, so WorkspaceID/UserID work unchanged.
// perMinute <= 0 disables rate limiting; limiter == nil (no Redis) or a
// limiter error falls back to a per-process window.
func APIKeyAuth(store APIKeyStore, limiter auth.Counter, perMinute int) func(http.Handler) http.Handler {
	local := NewQuota(perMinute, apiKeyRateWindow)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := apikey.Bearer(r.Header.Get("Authorization"))
			if !apikey.LooksLikeKey(token) {
				unauthorized(w, "missing or malformed API key")
				return
			}
			hash := apikey.Hash(token)
			key, err := store.GetActiveWorkspaceAPIKeyByHash(r.Context(), hash)
			if err != nil || subtle.ConstantTimeCompare([]byte(key.KeyHash), []byte(hash)) != 1 {
				unauthorized(w, "invalid API key")
				return
			}
			if retry, limited := overLimit(r.Context(), limiter, local, perMinute, key.ID); limited {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			go func() { // ponytail: one UPDATE per request; batch if this table gets hot
				tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = store.TouchWorkspaceAPIKey(tctx, database.TouchWorkspaceAPIKeyParams{ID: key.ID, WorkspaceID: key.WorkspaceID})
			}()

			claims := &auth.Claims{WorkspaceID: key.WorkspaceID, UserID: uuidString(key.CreatedBy), Name: key.Name, Role: "api_key"}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			ctx = context.WithValue(ctx, apiKeyScopesKey{}, key.Scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// overLimit uses the shared counter, degrading to the per-process window when
// it is absent or erroring (a Redis blip must not lock agents out, nor lift the limit).
func overLimit(ctx context.Context, limiter auth.Counter, local *Quota, perMinute int, keyID string) (time.Duration, bool) {
	if perMinute <= 0 {
		return 0, false
	}
	if limiter != nil {
		if n, ttl, err := limiter.Incr(ctx, "apikey_rl:"+keyID, apiKeyRateWindow); err == nil {
			return ttl, n > int64(perMinute)
		}
	}
	return apiKeyRateWindow, !local.allowLocal(keyID)
}

// RequireScope rejects API-key requests lacking scope (use after APIKeyAuth).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !apikey.HasScope(APIKeyScopes(r.Context()), scope) {
				http.Error(w, `{"error":"API key lacks the `+scope+` scope"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyScopes returns the scopes of the authenticating API key (nil for JWT sessions).
func APIKeyScopes(ctx context.Context) []string {
	s, _ := ctx.Value(apiKeyScopesKey{}).([]string)
	return s
}

// APIKeyPrincipal returns the workspace and scopes of an API-key-authenticated
// request, or "" when the request did not come through APIKeyAuth (unlike
// WorkspaceID, it never falls back to the dev workspace).
func APIKeyPrincipal(ctx context.Context) (string, []string) {
	scopes, ok := ctx.Value(apiKeyScopesKey{}).([]string)
	c := ClaimsFromContext(ctx)
	if !ok || c == nil {
		return "", nil
	}
	return c.WorkspaceID, scopes
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="leadecho"`)
	http.Error(w, `{"error":"`+msg+`"}`, http.StatusUnauthorized)
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
