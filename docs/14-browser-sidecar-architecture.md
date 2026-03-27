# 14 — Browser Sidecar Architecture

LeadEcho uses **browser sidecars** — lightweight HTTP microservices wrapping headless browsers — to crawl platforms that block API access or require authenticated sessions. Each sidecar exposes a uniform HTTP/JSON contract so the Go backend can swap between them seamlessly.

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Go API (monitor.go)                        │
│                                                                     │
│  tick() → for each keyword + platform:                             │
│                                                                     │
│    Reddit:   Pinchtab → Scrapling → unauthenticated .json API      │
│    Twitter:  Pinchtab → Scrapling                                  │
│    LinkedIn: Camoufox → Pinchtab → Scrapling                       │
│    HN:       direct Algolia API (no sidecar needed)                │
│    Dev.to:   direct API (no sidecar needed)                        │
│    Lobsters: direct RSS (no sidecar needed)                        │
│                                                                     │
└────────┬──────────────┬──────────────┬──────────────────────────────┘
         │              │              │
         ▼              ▼              ▼
   ┌───────────┐  ┌───────────┐  ┌───────────┐
   │ Pinchtab  │  │ Camoufox  │  │ Scrapling │
   │ :9867     │  │ :9868     │  │ :9869     │
   │ Chromium  │  │ Firefox   │  │ Chromium  │
   │ stealth   │  │ C++ spoof │  │ anti-bot  │
   └───────────┘  └───────────┘  └───────────┘
```

---

## Sidecars Comparison

| Feature | Pinchtab | Camoufox | Scrapling |
|---------|----------|----------|-----------|
| **Engine** | Chromium (Playwright) | Firefox (Camoufox) | Chromium (Playwright) |
| **Stealth level** | High (stealth mode) | Very High (C++ fingerprint spoofing) | High (StealthyFetcher anti-bot bypass) |
| **Best for** | Reddit, Twitter | LinkedIn | Fallback for all platforms |
| **Image** | `pinchtab/pinchtab:latest` | Custom (`camoufox-sidecar/`) | Custom (`scrapling-sidecar/`) |
| **Memory** | ~2 GB | ~512 MB | ~1 GB |
| **Port** | 9867 | 9868 | 9869 |
| **Language** | Go binary | Python + FastAPI | Python + FastAPI |
| **Docker profile** | `browser` | `browser-pro` | `browser-scrapling` |

---

## Uniform HTTP/JSON Contract

All three sidecars implement the **same API contract**, allowing the Go backend to use identical client code:

### Endpoints

| Method | Path | Body | Response | Description |
|--------|------|------|----------|-------------|
| `GET` | `/health` | — | `{"status":"ok"}` | Liveness check |
| `POST` | `/navigate` | `{"url":"..."}` | `{"ok":true}` | Navigate browser to URL |
| `GET` | `/text` | — | `{"text":"..."}` | Get page body text |
| `POST` | `/cookies` | `[{"name","value","domain","path"}]` | `{"ok":true}` | Inject session cookies |
| `POST` | `/evaluate` | `{"expression":"JS code"}` | `{"result":"..."}` | Execute JavaScript, return result |

### Authentication

All endpoints (except `/health`) require a bearer token:

```
Authorization: Bearer <SIDECAR_TOKEN>
```

### Scrapling Bonus Endpoint

Scrapling additionally exposes a higher-level `/scrape` endpoint for CSS-based extraction without JavaScript:

| Method | Path | Body | Response |
|--------|------|------|----------|
| `POST` | `/scrape` | `{"url","selector","fields","limit","cookies"}` | `{"results":[...]}` |

This avoids the navigate → wait → evaluate round-trip and works even on pages that block JS execution.

---

## Go Client Architecture

Each sidecar has a corresponding Go client in `backend/internal/browser/`:

```
browser/
├── pinchtab.go       # PinchtabClient
├── camoufox.go       # CamoufoxClient
├── scrapling.go      # ScraplingClient
└── (shared Cookie type)
```

All clients implement the same method set:

```go
Heartbeat(ctx)                          error
Navigate(ctx, url)                      error
GetText(ctx)                            (string, error)
InjectCookies(ctx, []Cookie)            error
EvaluateJS(ctx, script)                 (string, error)
```

`ScraplingClient` additionally provides:

```go
Scrape(ctx, ScrapeRequest)              ([]map[string]string, error)
```

### Cookie Type

```go
type Cookie struct {
    Name   string `json:"name"`
    Value  string `json:"value"`
    Domain string `json:"domain"`
    Path   string `json:"path"`
}
```

---

## Crawl Priority Chain (Fallback Logic)

The monitor dispatches crawlers in priority order. Each sidecar is **optional** — if its client is `nil` (env var not set), it's skipped. If a crawler returns `nil`, the next fallback is attempted.

### Reddit

```
1. Pinchtab (authenticated, no 429s)
   └─ returns nil? → try next
2. Scrapling (authenticated, anti-bot bypass)
   └─ returns nil? → try next
3. Unauthenticated .json API (rate-limited, 429 backoff)
```

### Twitter / X

```
1. Pinchtab (authenticated via session cookies)
   └─ not configured? → try next
2. Scrapling (authenticated, stealth browser)
   └─ not configured? → skip (no unauthenticated Twitter API)
```

### LinkedIn

```
1. Camoufox (C++ fingerprint spoofing — most aggressive anti-bot)
   └─ not configured? → try next
2. Pinchtab (Chromium stealth — decent for LinkedIn)
   └─ not configured? → try next
3. Scrapling (StealthyFetcher — anti-bot bypass)
   └─ not configured? → skip (no unauthenticated LinkedIn)
```

### HN, Dev.to, Lobsters, Indie Hackers

Direct API/RSS — no sidecar needed.

---

## Session Cookie Management

Authenticated crawling requires session cookies from logged-in browser sessions.

### Storage

- Cookies are stored in the `platform_accounts` table
- Column: `access_token_enc` (TEXT) — AES-encrypted cookie string
- One session per workspace per platform

### Encryption

```go
// Encrypt before storage
encrypted := crypto.Encrypt(encKey, "cookie1=value1; cookie2=value2")

// Decrypt before use
cookieStr, err := crypto.Decrypt(encKey, encrypted)
```

The encryption key is derived from `JWT_SECRET` (or `ENCRYPTION_KEY` if set) via `crypto.DeriveKey()`.

### Cookie Injection Flow

```
1. User pastes cookies in Dashboard → Browser Sessions page
2. Backend encrypts and stores in platform_accounts
3. Monitor loads session → decrypts → parseCookieString()
4. Injects cookies into sidecar via POST /cookies
5. Navigates to search URL with authenticated context
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/settings/sessions/{platform}` | Get session info (username, not the cookie) |
| `PUT` | `/api/v1/settings/sessions/{platform}` | Save encrypted session cookie |
| `DELETE` | `/api/v1/settings/sessions/{platform}` | Remove session |
| `POST` | `/api/v1/settings/sessions/{platform}/test` | Heartbeat test via sidecar |

---

## Crawl Workflow (per platform)

Every sidecar-based crawler follows the same pattern:

```
1. Load encrypted session cookie from database
2. Decrypt with AES key
3. Parse cookie string → []browser.Cookie
4. Inject cookies into sidecar (POST /cookies)
5. Navigate to search URL (POST /navigate)
6. Wait for page render (2-5 seconds depending on platform)
7. Extract data via JavaScript (POST /evaluate) with platform-specific JS
8. Parse JSON result into typed Go structs
9. Filter by keyword + negative terms (filterContent)
10. Insert new mentions (deduplicated by platform_id)
11. Return []mentionAlert for scoring pipeline
```

### JavaScript Extractors

Each platform has a JS snippet that extracts structured data from the rendered DOM:

**LinkedIn** (`linkedInExtractorJS`):
- Selector: `[data-chameleon-result-urn]`
- Fields: text, author, time (datetime attr), URL (activity link)

**Twitter/X** (`tweetExtractorJS`):
- Selector: `[data-testid="tweet"]`
- Fields: text (tweetText), author (User-Name), time, URL (status link)

**Reddit** (JSON API, no JS needed):
- Fetches `/r/{subreddit}/new.json?limit=25`
- Parses standard Reddit listing JSON

---

## Docker Deployment

### Start specific sidecars

```bash
# Pinchtab only (Reddit + Twitter)
docker compose --profile browser up -d

# Camoufox only (Pro LinkedIn)
docker compose --profile browser-pro up -d

# Scrapling only (fallback)
docker compose --profile browser-scrapling up -d

# All sidecars
docker compose --profile browser --profile browser-pro --profile browser-scrapling up -d
```

### Environment Variables

| Variable | Default | Sidecar | Description |
|----------|---------|---------|-------------|
| `PINCHTAB_URL` | `http://localhost:9867` | Pinchtab | Sidecar base URL |
| `PINCHTAB_TOKEN` | `changeme` | Pinchtab | Bearer token |
| `CAMOUFOX_URL` | (empty = disabled) | Camoufox | Sidecar base URL |
| `CAMOUFOX_TOKEN` | `changeme` | Camoufox | Bearer token |
| `SCRAPLING_URL` | (empty = disabled) | Scrapling | Sidecar base URL |
| `SCRAPLING_TOKEN` | `changeme` | Scrapling | Bearer token |

A sidecar is **enabled** when its `*_URL` env var is non-empty. If empty, the Go client is `nil` and skipped in the fallback chain.

### Resource Limits

| Sidecar | Memory | CPUs | Notes |
|---------|--------|------|-------|
| Pinchtab | 2 GB | 2.0 | Persistent Chromium, multiple tabs |
| Camoufox | 512 MB | — | Single-page stealth Firefox |
| Scrapling | 1 GB | — | On-demand browser per request |

---

## Health Monitoring

### Heartbeat Check

Each Go client exposes `Heartbeat(ctx) error`:

```go
if err := m.pinchtab.Heartbeat(ctx); err != nil {
    log.Warn().Err(err).Msg("pinchtab unreachable")
}
```

The dashboard's Browser Sessions page uses `POST /settings/sessions/{platform}/test` to verify sidecar connectivity.

### Docker Health

Add to `docker-compose.yml` for automatic restart:

```yaml
healthcheck:
  test: ["CMD", "curl", "-sf", "http://localhost:PORT/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

---

## Troubleshooting

### Sidecar not responding

```bash
# Check if container is running
docker ps | grep leadecho-pinchtab

# Check logs
docker logs leadecho-pinchtab --tail 50

# Test health endpoint directly
curl http://localhost:9867/health
```

### Session cookies expired

1. Platform will return login page instead of search results
2. JS extractor returns empty array
3. No new mentions appear for that platform
4. **Fix**: User re-pastes fresh cookies in Dashboard → Browser Sessions

### Anti-bot detection (LinkedIn)

- Symptom: Empty results, CAPTCHA pages
- Fix: Switch to Camoufox (higher stealth level)
- If Camoufox also fails: reduce crawl frequency, rotate sessions

### Memory issues

```bash
# Check memory usage
docker stats leadecho-pinchtab leadecho-camoufox leadecho-scrapling

# Restart a sidecar
docker compose --profile browser restart pinchtab
```

---

## Adding a New Sidecar

To add support for a new browser sidecar:

1. **Create the sidecar** in a new directory (e.g., `new-sidecar/`)
   - Implement the 5 endpoints: `/health`, `/navigate`, `/text`, `/cookies`, `/evaluate`
   - Add `Dockerfile` and `requirements.txt`

2. **Create Go client** in `backend/internal/browser/new_sidecar.go`
   - Mirror the `CamoufoxClient` struct and methods

3. **Add crawlers** in `backend/internal/monitor/*_new_sidecar.go`
   - Follow the pattern: load session → decrypt → inject → navigate → wait → extract → filter → insert

4. **Wire into Monitor** in `monitor.go`
   - Add field to `Monitor` struct
   - Add parameter to `New()` constructor
   - Add to fallback chain in `tick()`

5. **Add config** in `config/config.go`
   - `NewSidecarURL` and `NewSidecarToken` env vars

6. **Add Docker service** in `docker-compose.yml`
   - Use a profile (e.g., `browser-new`)

7. **Initialize in `main.go`**
   - Create client if URL is set, pass to `monitor.New()`
