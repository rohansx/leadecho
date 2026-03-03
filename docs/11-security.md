# LeadEcho - Security Best Practices

## Overview

LeadEcho handles sensitive data: API keys for social platforms, user credentials (via Clerk), payment information (via Stripe), and AI API keys. This document covers authentication, authorization, data protection, API security, and compliance requirements.

**Security principle:** Defense in depth. No single layer failure should compromise the system.

---

## Authentication

### Dashboard Authentication (Clerk JWT)

```go
// middleware/auth.go
package middleware

import (
    "net/http"
    "strings"

    "github.com/clerk/clerk-sdk-go/v2"
    clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
)

// ClerkAuth validates JWT tokens from the dashboard
func ClerkAuth() func(http.Handler) http.Handler {
    return clerkhttp.WithHeaderAuthorization()
}

// RequireAuth extracts and validates claims
func RequireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims, ok := clerk.SessionClaimsFromContext(r.Context())
        if !ok {
            respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "valid session required")
            return
        }

        // Extract workspace (org) from claims
        orgID := claims.ActiveOrganizationID
        if orgID == "" {
            respondError(w, http.StatusForbidden, "NO_WORKSPACE", "select a workspace")
            return
        }

        // Add to context for downstream handlers
        ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.Subject)
        ctx = context.WithValue(ctx, ctxKeyWorkspaceID, orgID)
        ctx = context.WithValue(ctx, ctxKeyRole, claims.ActiveOrganizationRole)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Chrome Extension Authentication (API Key)

```go
// middleware/apikey.go

func APIKeyAuth(db *database.Queries) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get("X-API-Key")
            if apiKey == "" {
                respondError(w, http.StatusUnauthorized, "MISSING_API_KEY", "X-API-Key header required")
                return
            }

            // Lookup by key prefix (first 8 chars stored in plaintext for lookup)
            prefix := apiKey[:8]
            keyRecord, err := db.GetAPIKeyByPrefix(r.Context(), prefix)
            if err != nil {
                respondError(w, http.StatusUnauthorized, "INVALID_API_KEY", "invalid API key")
                return
            }

            // Verify full key against bcrypt hash
            if err := bcrypt.CompareHashAndPassword([]byte(keyRecord.KeyHash), []byte(apiKey)); err != nil {
                respondError(w, http.StatusUnauthorized, "INVALID_API_KEY", "invalid API key")
                return
            }

            // Check if key is active
            if !keyRecord.IsActive {
                respondError(w, http.StatusForbidden, "KEY_DISABLED", "API key has been disabled")
                return
            }

            // Update last used timestamp (async)
            go db.UpdateAPIKeyLastUsed(context.Background(), keyRecord.ID)

            ctx := context.WithValue(r.Context(), ctxKeyUserID, keyRecord.UserID)
            ctx = context.WithValue(ctx, ctxKeyWorkspaceID, keyRecord.WorkspaceID)

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### API Key Generation

```go
func GenerateAPIKey() (plaintext string, hash string, prefix string, err error) {
    // Generate 32 random bytes
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", "", "", err
    }

    plaintext = "leh_" + base64.URLEncoding.EncodeToString(b) // e.g., leh_a3f8k2m...
    prefix = plaintext[:11] // "leh_a3f8k2m" (stored for lookup)

    hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
    if err != nil {
        return "", "", "", err
    }

    return plaintext, string(hashBytes), prefix, nil
}
```

---

## Authorization (RBAC)

### Role Hierarchy

| Role | Mentions | Replies | Keywords | Workflows | Settings | Billing |
|------|----------|---------|----------|-----------|----------|---------|
| Viewer | Read | Read | Read | Read | - | - |
| Editor | Read/Update | Read/Create/Update | CRUD | CRUD | - | - |
| Admin | Full | Full | Full | Full | Full | Full |

### Role Enforcement Middleware

```go
func RequireRole(roles ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userRole := r.Context().Value(ctxKeyRole).(string)

            allowed := false
            for _, role := range roles {
                if userRole == role {
                    allowed = true
                    break
                }
            }

            if !allowed {
                respondError(w, http.StatusForbidden, "INSUFFICIENT_ROLE",
                    fmt.Sprintf("requires one of: %s", strings.Join(roles, ", ")))
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

// Usage in router
r.With(RequireRole("admin")).Patch("/api/v1/team/members/{id}/role", h.UpdateMemberRole)
r.With(RequireRole("admin", "editor")).Post("/api/v1/keywords", h.CreateKeyword)
```

### Workspace Scoping

```go
// All database queries are automatically scoped by workspace ID
// This prevents cross-tenant data access

func (h *Handler) ListMentions(w http.ResponseWriter, r *http.Request) {
    workspaceID := getWorkspaceID(r) // From auth middleware context

    // Query is automatically scoped - no workspace_id parameter exposed to client
    mentions, err := h.db.ListMentions(r.Context(), database.ListMentionsParams{
        WorkspaceID: workspaceID, // Always from JWT claims, never from request
        // ... other filters from query params
    })

    respondJSON(w, http.StatusOK, mentions)
}
```

---

## Rate Limiting

### Redis-Backed Token Bucket

```go
// middleware/ratelimit.go

type RateLimiter struct {
    redis *redis.Client
    db    *database.Queries
}

func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            workspaceID := getWorkspaceID(r)

            // Get tier limits
            tier := rl.getTier(r.Context(), workspaceID)
            limit := rl.getLimitForTier(tier) // requests per minute

            // Token bucket in Redis
            key := fmt.Sprintf("ratelimit:api:%s", workspaceID)
            now := time.Now().Unix()

            // Lua script for atomic token bucket
            result, err := rl.redis.Eval(r.Context(), rateLimitScript, []string{key}, limit, now).Result()
            if err != nil {
                // Fail open on Redis errors
                next.ServeHTTP(w, r)
                return
            }

            remaining := result.(int64)

            w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
            w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
            w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(now+60, 10))

            if remaining < 0 {
                w.Header().Set("Retry-After", "60")
                respondError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

const rateLimitScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local window = 60

local current = redis.call('GET', key)
if current == false then
    redis.call('SET', key, limit - 1, 'EX', window)
    return limit - 1
end

local remaining = tonumber(current) - 1
redis.call('SET', key, remaining, 'KEEPTTL')
return remaining
`
```

---

## Webhook Signature Verification

### Clerk Webhook (Svix)

```go
func (h *Handler) HandleClerkWebhook(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // Verify Svix signature
    wh, err := svix.NewWebhook(h.clerkWebhookSecret)
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    headers := http.Header{}
    headers.Set("svix-id", r.Header.Get("svix-id"))
    headers.Set("svix-timestamp", r.Header.Get("svix-timestamp"))
    headers.Set("svix-signature", r.Header.Get("svix-signature"))

    if err := wh.Verify(body, headers); err != nil {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    // Process verified webhook
    var event ClerkWebhookEvent
    json.Unmarshal(body, &event)
    // ... handle user.created, organization.created, etc.
}
```

### Stripe Webhook

```go
func (h *Handler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
    if err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.stripeWebhookSecret)
    if err != nil {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    switch event.Type {
    case "checkout.session.completed":
        // ... handle subscription creation
    case "customer.subscription.updated":
        // ... handle plan changes
    case "customer.subscription.deleted":
        // ... handle cancellation
    case "invoice.payment_failed":
        // ... handle failed payment
    }

    w.WriteHeader(http.StatusOK)
}
```

### Outbound Webhook Signing

```go
func (h *WebhookSender) Sign(payload []byte, secret string) string {
    timestamp := strconv.FormatInt(time.Now().Unix(), 10)
    signedPayload := timestamp + "." + string(payload)

    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(signedPayload))
    signature := hex.EncodeToString(mac.Sum(nil))

    return fmt.Sprintf("t=%s,sha256=%s", timestamp, signature)
}
```

---

## Data Protection

### Encryption at Rest

- **PostgreSQL:** Railway managed encryption (AES-256)
- **Redis:** Railway managed encryption
- **API keys:** bcrypt-hashed before storage (never stored plaintext)
- **Secrets:** Railway encrypted environment variables

### Encryption in Transit

- All Railway services communicate over private network (TLS internal)
- Public endpoints enforce HTTPS (Railway automatic SSL)
- Database connections use `sslmode=require` in production

### PII Handling

```go
// Hash IP addresses before storing
func hashIP(ip string) string {
    h := sha256.New()
    h.Write([]byte(ip))
    return hex.EncodeToString(h.Sum(nil))[:16]
}

// Never store raw social media session cookies
// Chrome extension reads feed data only - no credential extraction
// LinkedIn content script operates on publicly visible feed data
```

### Credential Storage

| Credential | Storage | Notes |
|-----------|---------|-------|
| User passwords | Clerk (managed) | Never stored by LeadEcho |
| API keys | bcrypt hash in PostgreSQL | Prefix stored for lookup |
| Reddit OAuth tokens | Encrypted in PostgreSQL | AES-256 column encryption |
| Twitter bearer tokens | Environment variable | Never in code or DB |
| Stripe keys | Environment variable | Never in code or DB |
| AI API keys | Environment variable | Never in code or DB |

---

## Input Validation

### Request Validation

```go
// All user input is validated before processing
func validateCreateKeyword(req CreateKeywordRequest) error {
    if req.Term == "" {
        return fmt.Errorf("term is required")
    }
    if len(req.Term) > 100 {
        return fmt.Errorf("term must be <= 100 characters")
    }

    validPlatforms := map[string]bool{"reddit": true, "hackernews": true, "twitter": true, "linkedin": true}
    for _, p := range req.Platforms {
        if !validPlatforms[p] {
            return fmt.Errorf("invalid platform: %s", p)
        }
    }

    validMatchTypes := map[string]bool{"contains": true, "exact": true, "regex": true}
    if !validMatchTypes[req.MatchType] {
        return fmt.Errorf("invalid match_type: %s", req.MatchType)
    }

    return nil
}
```

### SQL Injection Prevention

```go
// sqlc generates parameterized queries - SQL injection is impossible
// All queries use $1, $2, etc. placeholders

// NEVER do this:
// query := fmt.Sprintf("SELECT * FROM mentions WHERE platform = '%s'", platform)

// sqlc generates this:
// SELECT * FROM mentions WHERE workspace_id = $1 AND platform = $2
```

### XSS Prevention

- Dashboard uses React which auto-escapes content by default
- `dangerouslySetInnerHTML` is never used
- Content Security Policy headers set

```go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        next.ServeHTTP(w, r)
    })
}
```

---

## Chrome Extension Security

### Manifest V3 Security Model

- **Minimal permissions:** Only `activeTab`, `sidePanel`, `storage`, `alarms`, `tabs`
- **Host permissions:** Only the 4 target platforms (LinkedIn, Reddit, X, Twitter)
- **No background page:** Service worker only (no persistent background scripts)
- **Content Security Policy:** Default MV3 CSP (no `eval()`, no inline scripts)

### Data Handling

```typescript
// NEVER extract or store:
// - Session cookies
// - Login credentials
// - Private messages
// - Data behind authentication walls

// ONLY extract from publicly visible feed:
// - Post content (text)
// - Author name and headline
// - Post URL and URN
// - Engagement counts (public)
```

### API Key Storage

```typescript
// Store API key in chrome.storage.local (encrypted by Chrome)
export async function setApiKey(key: string): Promise<void> {
    await chrome.storage.local.set({ apiKey: key });
}

export async function getApiKey(): Promise<string | null> {
    const result = await chrome.storage.local.get('apiKey');
    return result.apiKey || null;
}

// API key is transmitted over HTTPS only
// Never logged or exposed in content scripts
```

---

## AI Security

### Prompt Injection Prevention

```go
// User-provided content (mentions, documents) is clearly delimited
// in prompts to prevent injection attacks

func buildScoringPrompt(mentions []RawMention) string {
    var sb strings.Builder
    sb.WriteString("Score the following mentions for relevance.\n\n")

    for i, m := range mentions {
        // User content is wrapped in XML-style tags to clearly delineate boundaries
        sb.WriteString(fmt.Sprintf("### Mention %d\n", i+1))
        sb.WriteString(fmt.Sprintf("<user_content>\n%s\n</user_content>\n\n", m.Content))
    }

    return sb.String()
}
```

### Cost Controls

```go
// Per-workspace daily AI spend limits
type AIBudgetLimiter struct {
    redis *redis.Client
}

func (l *AIBudgetLimiter) CheckBudget(ctx context.Context, workspaceID string, estimatedCost float64) error {
    key := fmt.Sprintf("ai_spend:%s:%s", workspaceID, time.Now().Format("2006-01-02"))

    current, _ := l.redis.IncrByFloat(ctx, key, estimatedCost).Result()
    if current == estimatedCost {
        l.redis.Expire(ctx, key, 25*time.Hour)
    }

    limit := l.getDailyLimit(ctx, workspaceID) // Based on tier
    if current > limit {
        return fmt.Errorf("daily AI budget exceeded: $%.2f / $%.2f", current, limit)
    }

    return nil
}
```

---

## CORS Configuration

```go
func CORSConfig() func(http.Handler) http.Handler {
    return cors.Handler(cors.Options{
        AllowedOrigins: []string{
            "https://app.leadecho.app",
            "https://staging-app.leadecho.app",
        },
        AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Authorization", "Content-Type", "X-API-Key"},
        ExposedHeaders:   []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
        AllowCredentials: true,
        MaxAge:           300,
    })
}

// Chrome extension requests don't go through CORS
// (they use background service worker fetch, not content script)
```

---

## Pre-Launch Security Checklist

### Authentication & Authorization
- [ ] Clerk JWT validation on all dashboard endpoints
- [ ] API key validation on all extension endpoints
- [ ] RBAC enforcement on destructive operations
- [ ] Workspace scoping on all database queries
- [ ] No direct object reference vulnerabilities (IDOR)

### Data Protection
- [ ] All secrets in environment variables (not in code)
- [ ] API keys bcrypt-hashed in database
- [ ] HTTPS enforced on all endpoints
- [ ] PII hashed or excluded from logs
- [ ] Database connection uses SSL

### API Security
- [ ] Rate limiting per workspace tier
- [ ] Input validation on all endpoints
- [ ] SQL injection prevented (sqlc parameterized queries)
- [ ] XSS prevented (React auto-escaping, CSP headers)
- [ ] CORS configured for allowed origins only
- [ ] Security headers set (X-Frame-Options, etc.)
- [ ] Request body size limits enforced

### Webhook Security
- [ ] Clerk webhook signature verified (Svix)
- [ ] Stripe webhook signature verified
- [ ] Outbound webhooks HMAC-signed
- [ ] Webhook endpoints not exposed in public API docs

### Chrome Extension
- [ ] Minimal permissions in manifest
- [ ] No credential/cookie extraction
- [ ] API key stored in chrome.storage.local
- [ ] Content Security Policy enforced

### Infrastructure
- [ ] No secrets in Docker images
- [ ] Non-root container users
- [ ] Health check endpoints implemented
- [ ] Error messages don't leak internal details
- [ ] Dependency vulnerabilities scanned (Dependabot/Snyk)

### AI Security
- [ ] Prompt injection boundaries for user content
- [ ] Per-workspace AI spend limits
- [ ] AI output sanitized before display
- [ ] No sensitive data in AI prompts (no API keys, passwords)
