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
// limiter == nil or perMinute <= 0 disables rate limiting.
func APIKeyAuth(store APIKeyStore, limiter auth.Counter, perMinute int) func(http.Handler) http.Handler {
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
			if retry, limited := overLimit(r.Context(), limiter, perMinute, key.ID); limited {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			go func() { // ponytail: one UPDATE per request; batch if this table gets hot
				_ = store.TouchWorkspaceAPIKey(context.Background(), database.TouchWorkspaceAPIKeyParams{ID: key.ID, WorkspaceID: key.WorkspaceID})
			}()

			claims := &auth.Claims{WorkspaceID: key.WorkspaceID, UserID: uuidString(key.CreatedBy), Name: key.Name, Role: "api_key"}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			ctx = context.WithValue(ctx, apiKeyScopesKey{}, key.Scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// overLimit fails open when the counter backend errors: a Redis blip must not
// lock every agent out.
func overLimit(ctx context.Context, limiter auth.Counter, perMinute int, keyID string) (time.Duration, bool) {
	if limiter == nil || perMinute <= 0 {
		return 0, false
	}
	n, ttl, err := limiter.Incr(ctx, "apikey_rl:"+keyID, apiKeyRateWindow)
	if err != nil {
		return 0, false
	}
	return ttl, n > int64(perMinute)
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
