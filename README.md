# LeadEcho

**Find buyers before they find your competitors.**

LeadEcho monitors Reddit, Twitter/X, LinkedIn, Quora, Hacker News, Dev.to, Lobsters, and Indie Hackers for people actively describing the exact problem your product solves — then scores them by conversion probability, drafts AI replies from your knowledge base, and lets you post them with one click via the Chrome extension.

→ **[Live demo](https://app.leadecho.io)** · **[Product Hunt](https://www.producthunt.com/posts/leadecho)** · **[Docs](./docs/)**

---

## Features

- **Semantic pain-point matching** — Voyage AI embeddings match posts by meaning, not keyword strings
- **4-stage AI scoring** — spam filter → semantic match → LLM intent classification → lead qualification
- **Smart tiered inbox** — Leads Ready / Worth Watching / Informational, auto-ranked
- **Chrome extension + side panel** — passive signal collection, reply queue with one-click posting
- **Human-mimicry typing** — Gaussian keystroke timing so replies look natural
- **UTM attribution** — unique short links per reply, track clicks → signups → revenue
- **Knowledge base RAG** — AI replies grounded in your own docs, not hallucinated
- **Webhook notifications** — Slack, Discord, email alerts for high-intent mentions
- **CRM sync** — new leads, stage changes and conversions pushed to HubSpot, Attio, or any HMAC-signed webhook
- **ICP fit score** — every enriched lead gets a 0–100% fit score from profile match, intent, and GitHub signals
- **Multi-provider AI** — BYOK: NVIDIA Nemotron, DeepSeek, GLM (ZhipuAI), OpenAI, Anthropic, or any compatible API
- **Authenticated crawling** — Reddit, Twitter, LinkedIn, Quora via browser session cookies
- **Stealth browser fallback** — Scrapling anti-bot bypass as backup when Pinchtab/Camoufox unavailable
- **Full E2E test suite** — 86 Playwright tests covering auth, inbox, pipeline, keywords, profiles, analytics, and more
- **Astro landing page** — interactive 3D hero with React Three Fiber and Framer Motion

---

## Self-host in 5 minutes

### Prerequisites

- Docker + Docker Compose
- An AI provider API key: NVIDIA, DeepSeek, GLM (ZhipuAI), OpenAI, or Anthropic

### 1. Clone and configure

```bash
git clone https://github.com/your-org/leadecho
cd leadecho
cp .env.example .env
```

Open `.env` and set at minimum:

```env
JWT_SECRET=<random 32+ char string>
DEEPSEEK_API_KEY=sk-...          # or NVIDIA_API_KEY / GLM_API_KEY / OPENAI_API_KEY
RESEND_API_KEY=re_...            # optional — enables email verification, password reset, welcome emails
```

### 2. Start services

```bash
# Start Postgres + Redis
docker compose up -d

# Run database migrations
make migrate-up

# Build and start API + dashboard
make up
```

| Service | URL |
|---|---|
| Dashboard | http://localhost:3100 |
| API | http://localhost:8090 |
| Postgres | localhost:5433 |
| Redis | localhost:6380 |

### 3. Register and onboard

1. Go to http://localhost:3100 and click **Get Started**
2. Register with email/password
3. Follow the 4-step onboarding wizard to set up pain-point profiles and keywords
4. The monitor starts scanning Reddit, HN, Dev.to, Lobsters, and Indie Hackers automatically every 5 minutes

---

## Platform setup

### Reddit & Hacker News (no setup)
Work out of the box using public APIs. No account needed.

### Dev.to, Lobsters, Indie Hackers (no setup)
Also public — no authentication required.

### Quora, Twitter/X and LinkedIn (authenticated)

These require the Pinchtab browser sidecar:

```bash
# Start with browser sidecar (needs PINCHTAB_TOKEN in .env)
docker compose --profile browser up -d
```

Then in the dashboard, go to **Settings → Browser Sessions** and paste your:
- **Twitter/X**: session cookie (`auth_token` from browser devtools)
- **LinkedIn**: `li_at` cookie
- **Quora**: session cookies (the sidecar auto-validates login state)

### LinkedIn stealth mode (Pro)

For less aggressive LinkedIn detection, use the Camoufox sidecar:

```bash
CAMOUFOX_URL=http://localhost:9868 docker compose --profile browser-pro up -d
```

### Anti-bot bypass fallback (Scrapling)

When Pinchtab and Camoufox are both unavailable, Scrapling provides a lightweight fallback:

```bash
docker compose --profile browser-scrapling up -d
```

---

## CRM & webhooks

Configure in **Settings → CRM & Webhooks**. Events: `lead.created`, `lead.stage_changed`, `conversion.recorded`.

- **HubSpot**: private app token with `crm.objects.contacts.write`. New leads become contacts; stages map to `lifecyclestage` (prospect→lead, qualified→SQL, engaged→opportunity, converted→customer).
- **Attio**: workspace token with record + note read-write scopes. New leads become People; stage changes and conversions are added as notes.
- **Signed webhook** (Zapier, Make, n8n, your own endpoint): HTTPS only. Verify each request:

```text
X-LeadEcho-Timestamp: 1700000000
X-LeadEcho-Signature: sha256=<hex HMAC-SHA256(secret, timestamp + "." + raw_body)>
```

Reject timestamps older than ~5 minutes. Delivery is best-effort (no retries yet).

---

## Chrome Extension

Install from the Chrome Web Store (link coming) or load unpacked:

```bash
cd extension
pnpm install
pnpm build
# Load the dist/ folder in chrome://extensions → "Load unpacked"
```

Then in the extension Settings tab:
- **Backend URL**: `http://localhost:8090` (or your hosted URL)
- **Extension Key**: generate one in the dashboard under **Settings → Extension Token**

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│  dashboard/ (React, Vite, TanStack Router)      │
│  landing/   (Astro, React, Three.js)             │
│  extension/ (WXT + MV3 Chrome extension)         │
└────────────────┬────────────────────────────────┘
                 │ HTTP
┌────────────────▼────────────────────────────────┐
│  backend/ (Go, Chi, pgx/v5)                     │
│  ├─ api/          HTTP handlers + middleware     │
│  ├─ auth/         JWT + Google OAuth             │
│  ├─ monitor/      9 crawlers (8 platforms + web-wide Exa) │
│  │   └─ scorer/   4-stage AI scoring pipeline    │
│  ├─ browser/      Pinchtab + Camoufox clients    │
│  ├─ embedding/    Voyage AI client               │
│  └─ database/     sqlc-generated queries         │
└────────────────┬────────────────────────────────┘
                 │
   ┌─────────────┴──────────────┐
   │ PostgreSQL (pgvector)      │
   │ Redis (session cache)      │
   └────────────────────────────┘
```

### Monitor dispatch (every 5 minutes)

| Platform | Method | Fallback |
|---|---|---|
| Reddit | Pinchtab (authenticated) | Public JSON API |
| Twitter/X | Pinchtab | — |
| LinkedIn | Camoufox → Pinchtab | — |
| Quora | Pinchtab (authenticated) | — |
| Web (Exa) | Exa semantic search (needs `EXA_API_KEY`) | — |
| HN, Dev.to, Lobsters, IH | Direct API | — |

### 4-stage scoring pipeline

1. **Rules filter** — minimum length, spam patterns
2. **Semantic match** — Voyage AI cosine similarity against pain-point profiles (threshold: 0.40)
3. **Intent classification** — LLM labels: `buy_signal`, `recommendation_ask`, `complaint`, `comparison`, `general`
4. **Lead qualification** — auto-creates lead for score ≥ 7.0

---

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | ✓ | PostgreSQL connection string |
| `REDIS_URL` | ✓ | Redis connection string |
| `JWT_SECRET` | ✓ | JWT signing secret (32+ chars) |
| `DEEPSEEK_API_KEY` | one of | DeepSeek key for AI scoring |
| `NVIDIA_API_KEY` | one of | NVIDIA Nemotron key for AI scoring |
| `GLM_API_KEY` | one of | GLM (ZhipuAI) key for AI scoring |
| `OPENAI_API_KEY` | one of | OpenAI key for AI scoring (also used for Anthropic-compatible) |
| `VOYAGE_API_KEY` | | Voyage AI for embeddings (falls back to keyword matching) |
| `EXA_API_KEY` | | Exa web-wide discovery source (add `exa` to a keyword's platforms; skipped if unset) |
| `RESEND_API_KEY` | | Resend for auth + notification emails. When set, password accounts must verify their email and password reset works; when unset, new accounts are auto-verified |
| `PINCHTAB_TOKEN` | | Pinchtab browser sidecar token |
| `PINCHTAB_URL` | | Pinchtab URL (default: http://localhost:9867) |
| `CAMOUFOX_URL` | | Camoufox stealth browser URL (Pro LinkedIn) |
| `CAMOUFOX_TOKEN` | | Camoufox auth token |
| `SCRAPLING_URL` | | Scrapling anti-bot fallback URL |
| `SCRAPLING_TOKEN` | | Scrapling auth token |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | | Enables "Continue with Google" |
| `GOOGLE_REDIRECT_URL` | | Default `FRONTEND_URL/api/v1/auth/google/callback` (register it in Google Cloud) |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | | Enables "Continue with GitHub" |
| `GITHUB_REDIRECT_URL` | | Default `FRONTEND_URL/api/v1/auth/github/callback` (register it in the GitHub OAuth app) |
| `FRONTEND_URL` | | Public app URL, used for OAuth redirects and email links (default: http://localhost:3100) |
| `TRUSTED_PROXY_HOPS` | | Number of reverse proxies in front of the API whose `X-Forwarded-For` is trusted for client IPs (rate limits, logs). Default `0` (never trust it). Count each proxy that appends the header: `1` for Traefik → API, `2` for Traefik → dashboard nginx → API |
| `PORT` | | API port (default: 8090) |
| `ENVIRONMENT` | | `development` or `production` |

---

## Production checklist

Before the first deploy (and after pulling a release that adds settings):

1. `ENVIRONMENT=production`. The API then refuses to boot without a strong `JWT_SECRET` and a different `ENCRYPTION_KEY` (`openssl rand -hex 32` for each). Changing `ENCRYPTION_KEY` later makes stored BYOK keys and CRM tokens unreadable.
2. `FRONTEND_URL` set to the public https URL. OAuth callbacks and tracked short links are built from it.
3. `TRUSTED_PROXY_HOPS` matching your proxy chain (see the table above). Too low: everyone shares one rate-limit bucket. Too high: clients can spoof their IP.
4. `RESEND_API_KEY`, with the sending domain verified in Resend, so verification and password-reset emails are sent.
5. Google and/or GitHub OAuth apps, each with the callback `<FRONTEND_URL>/api/v1/auth/<provider>/callback`.
6. An LLM key (`ANTHROPIC_API_KEY` or another provider) and an embeddings key (`VOYAGE_API_KEY`, or an OpenAI/GLM key in Settings), or the knowledge base and pain-point matching stay off.
7. Migrations run automatically through the `migrate` service; the API waits for it, so a failed migration shows up as an API that never starts. Check the `migrate` logs first.

## Development

```bash
# Install all dependencies
make install

# Start everything (DB + API + dashboard)
make up

# Watch logs
make logs

# Run migrations
make migrate-up

# Generate sqlc types after SQL changes
make sqlc

# Type check frontend
make web-check

# Run Go tests
make api-test

# Run E2E tests (Playwright — 86 tests)
make e2e

# Format code
make fmt
```

---

## Tech stack

| Layer | Tech |
|---|---|
| Backend | Go 1.23, Chi router, pgx/v5, sqlc, zerolog |
| Database | PostgreSQL 16 + pgvector |
| Cache | Redis 7 |
| Frontend | React 18, Vite, TanStack Router, TanStack Query, Tailwind |
| Landing | Astro 4, React 18, Three.js, Framer Motion |
| Extension | WXT 0.19, MV3, React |
| AI | NVIDIA Nemotron, DeepSeek, GLM (ZhipuAI), OpenAI, Anthropic (BYOK); Voyage AI (embeddings) |
| Email | Resend |
| Browser automation | Pinchtab, Camoufox, Scrapling |
| Testing | Playwright (86 E2E tests)

---

## License

MIT — see [LICENSE](./LICENSE)

---

## Contributing

Issues and PRs welcome. Please open an issue first for substantial changes.
