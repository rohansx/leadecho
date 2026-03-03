# LeadEcho

**Find buyers before they find your competitors.**

LeadEcho monitors Reddit, Twitter/X, LinkedIn, Hacker News, Dev.to, Lobsters, and Indie Hackers for people actively describing the exact problem your product solves — then scores them by conversion probability, drafts AI replies from your knowledge base, and lets you post them with one click via the Chrome extension.

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
- **BYOK** — bring your own OpenAI, Anthropic, or compatible API key
- **Authenticated crawling** — Reddit, Twitter, LinkedIn via browser session cookies

---

## Self-host in 5 minutes

### Prerequisites

- Docker + Docker Compose
- An OpenAI or Anthropic API key (for AI scoring)

### 1. Clone and configure

```bash
git clone https://github.com/your-org/leadecho
cd leadecho
cp .env.example .env
```

Open `.env` and set at minimum:

```env
JWT_SECRET=<random 32+ char string>
OPENAI_API_KEY=sk-...          # or ANTHROPIC_API_KEY
RESEND_API_KEY=re_...          # optional — for welcome emails
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
4. The monitor starts scanning Reddit + HN automatically every 5 minutes

---

## Platform setup

### Reddit & Hacker News (no setup)
Work out of the box using public APIs. No account needed.

### Dev.to, Lobsters, Indie Hackers (no setup)
Also public — no authentication required.

### Twitter/X and LinkedIn (authenticated)

These require the Pinchtab browser sidecar:

```bash
# Start with browser sidecar (needs PINCHTAB_TOKEN in .env)
docker compose --profile browser up -d
```

Then in the dashboard, go to **Settings → Browser Sessions** and paste your:
- **Twitter/X**: session cookie (`auth_token` from browser devtools)
- **LinkedIn**: `li_at` cookie

### LinkedIn stealth mode (Pro)

For less aggressive LinkedIn detection, use the Camoufox sidecar:

```bash
CAMOUFOX_URL=http://localhost:9868 docker compose --profile browser-pro up -d
```

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
│  landing/   (static HTML marketing site)        │
│  extension/ (WXT + MV3 Chrome extension)        │
└────────────────┬────────────────────────────────┘
                 │ HTTP
┌────────────────▼────────────────────────────────┐
│  backend/ (Go, Chi, pgx/v5)                     │
│  ├─ api/          HTTP handlers + middleware     │
│  ├─ auth/         JWT + Google OAuth             │
│  ├─ monitor/      9 platform crawlers           │
│  │   └─ scorer/   4-stage AI scoring pipeline   │
│  ├─ browser/      Pinchtab + Camoufox clients   │
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
| `OPENAI_API_KEY` | one of | OpenAI key for AI scoring |
| `ANTHROPIC_API_KEY` | one of | Anthropic key for AI scoring |
| `VOYAGE_API_KEY` | | Voyage AI for embeddings (falls back to keyword matching) |
| `RESEND_API_KEY` | | Resend for welcome + notification emails |
| `PINCHTAB_TOKEN` | | Pinchtab browser sidecar token |
| `PINCHTAB_URL` | | Pinchtab URL (default: http://localhost:9867) |
| `CAMOUFOX_URL` | | Camoufox stealth browser URL (Pro LinkedIn) |
| `CAMOUFOX_TOKEN` | | Camoufox auth token |
| `GOOGLE_CLIENT_ID` | | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | | Google OAuth client secret |
| `FRONTEND_URL` | | Frontend URL for OAuth redirects (default: http://localhost:3100) |
| `PORT` | | API port (default: 8090) |
| `ENVIRONMENT` | | `development` or `production` |

---

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
| Extension | WXT 0.19, MV3, React |
| AI | OpenAI / Anthropic (BYOK), Voyage AI (embeddings) |
| Email | Resend |
| Browser automation | Pinchtab, Camoufox |

---

## License

MIT — see [LICENSE](./LICENSE)

---

## Contributing

Issues and PRs welcome. Please open an issue first for substantial changes.
