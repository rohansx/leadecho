# LeadEcho

**Open-source social intent monitoring.** Find buying signals across Reddit and Hacker News, classify intent with AI, draft context-aware replies, and track conversions — all from a single dashboard.

```
Monitor → Classify → Engage → Convert
```

---

## Table of Contents

- [Why LeadEcho](#why-leadecho)
- [Features](#features)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Quick Start](#quick-start)
- [Environment Variables](#environment-variables)
- [Project Structure](#project-structure)
- [Database Schema](#database-schema)
- [API Reference](#api-reference)
- [Signal Engine (Monitor)](#signal-engine)
- [AI Pipeline](#ai-pipeline)
- [Frontend (Dashboard)](#frontend-dashboard)
- [Makefile Commands](#makefile-commands)
- [Deployment](#deployment)
- [Roadmap](#roadmap)
- [License](#license)

---

## Why LeadEcho

The social listening market ($10B+ in 2025) is saturated with "alert" tools — they tell you "someone mentioned your keyword!" then leave you to manually find the post, write a reply, and track nothing.

**LeadEcho closes the full loop:** monitor → engage → convert → measure.

| What exists today | What LeadEcho does differently |
|---|---|
| Alert-only tools ($15-49/mo) | Full pipeline: alerts + AI classification + reply drafting + lead tracking |
| Enterprise suites ($5K+/mo) | Open-source, self-hostable, free forever |
| No OSS social monitoring tool | First open-source social intent orchestrator |
| Manual cross-platform checking | Unified inbox across Reddit, HN (Twitter, LinkedIn planned) |
| Zero conversion tracking | Mention → reply → lead → conversion attribution |

### Who It's For

- **Indie hackers / solopreneurs** — stop manually checking Reddit daily for leads
- **Startup sales teams** — monitor buying signals across platforms from one dashboard
- **DevTool companies** — be in developer conversations on HN/Reddit automatically
- **Marketing agencies** — manage social monitoring for multiple clients

---

## Features

### Core (Implemented)

- **Keyword monitoring** — configure terms with platform selection, match types (broad/exact/phrase), negative terms, and specific subreddits
- **Hacker News crawler** — polls Algolia search API for stories and comments matching your keywords
- **Reddit crawler** — monitors specific subreddits via JSON feeds, filters locally by keyword (no auth required)
- **AI intent classification** — classifies mentions as `buy_signal`, `complaint`, `recommendation_ask`, `comparison`, or `general` with conversion probability scoring
- **AI reply drafting** — generates natural, platform-appropriate replies using your knowledge base context
- **Unified inbox** — all mentions in one place with status management (new → reviewed → replied → archived)
- **Lead pipeline** — convert mentions to leads, track through stages (prospect → qualified → engaged → converted)
- **Knowledge base (RAG)** — upload docs to train AI replies on your product's voice and features
- **Analytics dashboard** — mentions/day, platform breakdown, intent distribution, top keywords, conversion funnel
- **Notifications** — Slack, Discord, and email (via Resend) alerts on new mentions and high-intent signals
- **Auth** — email/password + Google OAuth, JWT sessions
- **Content filtering** — negative terms, match type enforcement, minimum content length to reduce noise

### Planned (Roadmap)

- Twitter/X monitoring (API access required)
- LinkedIn monitoring (via Chrome extension)
- Auto-posting replies (not just drafting)
- Chrome extension for in-platform engagement
- Workflow automation engine
- UTM link tracking (mention → click → signup → revenue)
- Team collaboration / multi-seat
- Multi-workspace (Pro plan)
- BYOK API keys (Pro plan)
- Stripe billing integration

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                      Go Backend                          │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │  Chi Router  │  │   Monitor    │  │   AI Module   │  │
│  │  + Middleware │  │   (Signal    │  │  (GLM / GPT)  │  │
│  │  + Handlers  │  │    Engine)   │  │  Classify +   │  │
│  │              │  │  HN + Reddit │  │  Draft Reply  │  │
│  └──────┬───────┘  └──────┬───────┘  └───────┬───────┘  │
│         │                 │                   │          │
│  ┌──────▼─────────────────▼───────────────────▼───────┐  │
│  │                   PostgreSQL 16                    │  │
│  │    pgvector │ Full-text search │ JSONB │ Enums     │  │
│  └────────────────────────┬───────────────────────────┘  │
│                           │                              │
│  ┌────────────────────────▼───────────────────────────┐  │
│  │                     Redis 7                        │  │
│  │              Cache │ Rate Limiting                 │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────┬───────────────────────────────┘
                           │ REST API
┌──────────────────────────▼───────────────────────────────┐
│                  Vite + React Dashboard                   │
│   TanStack Router │ TanStack Query │ Tailwind │ Zustand  │
│                                                          │
│   Inbox │ Pipeline │ Analytics │ Keywords │ Alerts │ KB  │
└──────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. User configures keywords + subreddits in dashboard
2. Monitor polls HN (Algolia API) and Reddit (/r/{sub}/new.json) every 5 minutes
3. Posts are filtered: negative terms, match type, min content length
4. Matching posts inserted as mentions (deduplicated by platform_id)
5. Notifications sent to configured Slack/Discord/Email channels
6. User reviews mentions in inbox, clicks "Classify" → AI scores intent
7. User clicks "Draft Reply" → AI generates platform-appropriate reply using KB context
8. User promotes mention to lead → tracks through pipeline stages
```

---

## Tech Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| **Backend** | Go 1.23+ | Goroutines for concurrent polling, single binary, low memory |
| **Router** | Chi v5 | stdlib-compatible, lightweight, great middleware ecosystem |
| **Database** | PostgreSQL 16 (pgvector) | Vector search, full-text search, JSONB, materialized views |
| **Cache** | Redis 7 | Rate limiting, caching, future pub/sub |
| **SQL** | sqlc | Type-safe Go code generated from SQL queries |
| **Migrations** | goose v3 | Simple, Go-native, SQL-first migrations |
| **Frontend** | Vite + React 19 | Fast builds, modern React with concurrent features |
| **Routing** | TanStack Router | File-based routing, type-safe, code-splitting |
| **Server State** | TanStack Query v5 | Cache invalidation, optimistic updates |
| **Client State** | Zustand v5 | Minimal boilerplate store |
| **Styling** | Tailwind CSS v4 | Rapid prototyping, neobrutalism design system |
| **UI** | Custom (CVA + Radix) | 2px borders, hard shadows, neobrutalist aesthetic |
| **Icons** | Lucide React | Consistent, tree-shakeable icon set |
| **AI** | Zhipu GLM-4.5 / OpenAI GPT-4o-mini | Intent classification + reply generation |
| **Embeddings** | Voyage AI (voyage-3) | 1024-dim vectors for RAG knowledge base |
| **Email** | Resend | Developer-friendly, 3K emails/mo free tier |
| **Auth** | JWT + bcrypt | Email/password + Google OAuth |

---

## Quick Start

### Prerequisites

- Go 1.23+
- Node.js 22 LTS + pnpm 9+
- Docker + Docker Compose

### Setup

```bash
# Clone
git clone https://github.com/your-org/leadecho.git
cd leadecho

# Install dependencies
make install

# Copy environment config
cp .env.example .env
# Edit .env with your API keys (GLM_API_KEY, RESEND_API_KEY, etc.)

# Start everything (Postgres, Redis, API, Dashboard)
make up
```

Dashboard: http://localhost:3100
API: http://localhost:8090

### First Steps

1. Register an account at http://localhost:3100/register
2. Go to **Keywords** — add a keyword (e.g., "CRM alternative"), select platforms, add subreddits to monitor
3. Go to **Alerts** — configure Slack/Discord/Email notifications
4. Go to **Knowledge Base** — upload your product docs so AI replies reference your product
5. Wait for the monitor to crawl (runs every 5 minutes) — mentions appear in **Inbox**
6. Click **Classify** on a mention to get AI intent scoring
7. Click **Draft Reply** to generate a context-aware reply

---

## Environment Variables

```bash
# === Required ===
DATABASE_URL=postgres://leadecho:leadecho@localhost:5433/leadecho_dev?sslmode=disable
REDIS_URL=redis://localhost:6380/0
JWT_SECRET=change-this-to-a-random-secret-in-production

# === Server ===
PORT=8090                    # API server port (default: 8090)
ENVIRONMENT=development      # development | staging | production
LOG_LEVEL=debug              # debug | info | warn | error
FRONTEND_URL=http://localhost:3100

# === Google OAuth (optional) ===
GOOGLE_CLIENT_ID=your-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-secret
GOOGLE_REDIRECT_URL=http://localhost:8090/api/v1/auth/google/callback

# === AI (at least one required for classify/draft-reply) ===
GLM_API_KEY=your-zhipu-api-key        # Zhipu AI (free tier available)
# OPENAI_API_KEY=your-openai-key      # Fallback if GLM not set

# === Embeddings (required for knowledge base RAG) ===
VOYAGE_API_KEY=your-voyage-key

# === Email notifications (optional) ===
RESEND_API_KEY=re_xxxx
```

---

## Project Structure

```
leadecho/
├── backend/
│   ├── cmd/api/main.go              # Entry point — starts server + monitor
│   ├── internal/
│   │   ├── api/
│   │   │   ├── router.go            # Chi route definitions
│   │   │   ├── handler/             # HTTP handlers (mentions, leads, keywords, AI, etc.)
│   │   │   └── middleware/           # Auth, CORS, logging, request ID
│   │   ├── ai/ai.go                 # LLM integration (classify + draft reply)
│   │   ├── auth/                    # Email auth + Google OAuth
│   │   ├── config/config.go         # Environment variable loading
│   │   ├── crypto/                  # Encryption utilities
│   │   ├── database/
│   │   │   ├── queries/             # .sql files (sqlc source)
│   │   │   ├── models.go            # Generated Go models
│   │   │   └── *.sql.go             # Generated query functions
│   │   └── monitor/
│   │       ├── monitor.go           # Main polling loop (all workspaces)
│   │       ├── hackernews.go        # HN Algolia API crawler
│   │       ├── reddit.go            # Reddit subreddit feed crawler
│   │       ├── filter.go            # Content relevance filtering
│   │       └── notify.go            # Slack/Discord/Email notifications
│   ├── migrations/                  # goose SQL migrations
│   ├── sqlc.yaml                    # sqlc configuration
│   ├── go.mod
│   └── Dockerfile
├── dashboard/
│   ├── src/
│   │   ├── routes/
│   │   │   ├── _auth/               # Login + Register pages
│   │   │   ├── _dashboard/
│   │   │   │   ├── inbox.tsx         # Mention inbox with classify + draft reply
│   │   │   │   ├── pipeline.tsx      # Lead CRM pipeline (Kanban-style)
│   │   │   │   ├── analytics.tsx     # Charts + metrics dashboard
│   │   │   │   ├── keywords.tsx      # Keyword management + subreddit config
│   │   │   │   ├── alerts.tsx        # Notification channel configuration
│   │   │   │   ├── knowledge-base.tsx # RAG document management
│   │   │   │   ├── workflows.tsx     # Workflow automation (coming soon)
│   │   │   │   └── settings.tsx      # Account settings
│   │   │   └── __root.tsx
│   │   ├── components/
│   │   │   ├── ui/                   # Base components (Button, Card, Badge, etc.)
│   │   │   └── layout/sidebar.tsx    # Navigation sidebar
│   │   └── lib/
│   │       ├── api.ts                # API client (fetch wrapper)
│   │       └── types.ts              # TypeScript interfaces
│   ├── package.json
│   └── Dockerfile
├── docs/                             # Architecture docs (you are here)
├── docker-compose.yml                # Local Postgres + Redis
├── Makefile                          # Dev commands
└── .env.example                      # Environment template
```

---

## Database Schema

PostgreSQL 16 with pgvector extension. All queries managed via sqlc. Migrations via goose.

### Entity Relationship

```
workspaces ──┬── users
             ├── keywords (+ subreddits)
             ├── mentions ──┬── threads
             │              ├── replies ── utm_links
             │              └── leads ── lead_events
             ├── documents ── document_chunks (pgvector 1024-dim)
             ├── workflows ── workflow_executions
             ├── platform_accounts
             ├── notifications
             └── subscriptions ── subscription_plans
```

### Enums

| Enum | Values |
|------|--------|
| `platform_type` | hackernews, reddit, twitter, linkedin |
| `mention_status` | new, reviewed, replied, archived, spam |
| `intent_type` | buy_signal, complaint, recommendation_ask, comparison, general |
| `lead_stage` | prospect, qualified, engaged, converted, lost |
| `reply_status` | draft, approved, posted, failed |
| `reply_variant` | value_only, technical, soft_sell |
| `workflow_status` | active, paused, disabled |
| `user_role` | admin, editor, viewer |

### Core Tables

#### `keywords`

```sql
CREATE TABLE keywords (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id),
    term            TEXT NOT NULL,
    platforms       platform_type[] NOT NULL DEFAULT '{hackernews,reddit,twitter,linkedin}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    match_type      TEXT NOT NULL DEFAULT 'contains',   -- broad | exact | phrase
    negative_terms  TEXT[] NOT NULL DEFAULT '{}',
    subreddits      TEXT[] NOT NULL DEFAULT '{}',        -- e.g. {"SaaS","startups"}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, term)
);
```

#### `mentions`

```sql
CREATE TABLE mentions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id            UUID NOT NULL REFERENCES workspaces(id),
    keyword_id              UUID REFERENCES keywords(id),
    platform                platform_type NOT NULL,
    platform_id             TEXT NOT NULL,
    url                     TEXT NOT NULL,
    title                   TEXT,
    content                 TEXT NOT NULL,
    content_tsv             TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    author_username         TEXT,
    author_profile_url      TEXT,
    relevance_score         REAL,
    intent                  intent_type,
    conversion_probability  REAL,
    status                  mention_status NOT NULL DEFAULT 'new',
    platform_metadata       JSONB NOT NULL DEFAULT '{}',
    engagement_metrics      JSONB NOT NULL DEFAULT '{}',
    keyword_matches         TEXT[] NOT NULL DEFAULT '{}',
    platform_created_at     TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, platform, platform_id)
);
```

#### `leads`

```sql
CREATE TABLE leads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id),
    mention_id      UUID REFERENCES mentions(id),
    stage           lead_stage NOT NULL DEFAULT 'prospect',
    contact_name    TEXT,
    contact_email   TEXT,
    company         TEXT,
    username        TEXT,
    platform        platform_type,
    profile_url     TEXT,
    estimated_value INTEGER DEFAULT 0,
    notes           TEXT,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `documents` + `document_chunks`

```sql
CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id),
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    content_type    TEXT NOT NULL DEFAULT 'markdown',
    chunk_count     INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE document_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id),
    workspace_id    UUID NOT NULL,
    content         TEXT NOT NULL,
    embedding       vector(1024) NOT NULL,  -- voyage-3 embeddings
    chunk_index     INTEGER NOT NULL,
    token_count     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- HNSW index for fast vector similarity search
CREATE INDEX idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

### Migrations

| File | Description |
|------|-------------|
| `00001_init_schema.sql` | Full schema: all tables, enums, indexes, triggers |
| `00002_seed_dev_data.sql` | Development seed data |
| `00003_remove_voice_features.sql` | Remove unused voice/call features |
| `00004_add_password_hash.sql` | Add email/password auth support |
| `00005_add_subreddits.sql` | Add subreddits column to keywords |

---

## API Reference

Base URL: `http://localhost:8090/api/v1`

All protected routes require a JWT cookie (set by login/register).

### Auth

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/auth/register` | Register with email + password |
| `POST` | `/auth/login` | Login with email + password |
| `GET` | `/auth/google` | Initiate Google OAuth flow |
| `GET` | `/auth/google/callback` | OAuth callback (sets JWT cookie) |
| `GET` | `/auth/me` | Get current user info |
| `POST` | `/auth/logout` | Clear session |

### Mentions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/mentions` | List mentions (filters: status, platform, intent, search, limit, offset) |
| `GET` | `/mentions/{id}` | Get single mention |
| `GET` | `/mentions/counts` | Aggregate counts by status |
| `PATCH` | `/mentions/{id}/status` | Update status (new/reviewed/replied/archived/spam) |
| `POST` | `/mentions/{id}/classify` | AI intent classification |
| `POST` | `/mentions/{id}/draft-reply` | AI reply generation (uses KB context) |

### Keywords

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/keywords` | List all keywords |
| `POST` | `/keywords` | Create keyword |
| `GET` | `/keywords/{id}` | Get keyword |
| `PUT` | `/keywords/{id}` | Update keyword |
| `DELETE` | `/keywords/{id}` | Delete keyword |

**Create keyword body:**

```json
{
  "term": "CRM alternative",
  "platforms": ["reddit", "hackernews"],
  "match_type": "broad",
  "negative_terms": ["hiring", "job"],
  "subreddits": ["SaaS", "startups", "smallbusiness"]
}
```

### Leads

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/leads` | List leads (filters: stage, limit, offset) |
| `POST` | `/leads` | Create lead from mention |
| `GET` | `/leads/{id}` | Get lead |
| `GET` | `/leads/counts` | Pipeline stage counts |
| `PATCH` | `/leads/{id}/stage` | Update lead stage |

### Replies

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/mentions/{mentionId}/replies` | List replies for a mention |
| `POST` | `/replies` | Create reply draft |
| `PATCH` | `/replies/{id}/content` | Edit reply content |
| `PATCH` | `/replies/{id}/status` | Update status (draft/approved/posted) |

### Documents (Knowledge Base)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/documents` | List documents |
| `POST` | `/documents` | Upload document |
| `GET` | `/documents/{id}` | Get document |
| `PUT` | `/documents/{id}` | Update document |
| `DELETE` | `/documents/{id}` | Delete document |

### Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/analytics/overview` | Dashboard summary (mentions, leads, replies, keywords) |
| `GET` | `/analytics/mentions-per-day` | Time series (last 30 days) |
| `GET` | `/analytics/mentions-per-platform` | Platform breakdown |
| `GET` | `/analytics/mentions-per-intent` | Intent distribution |
| `GET` | `/analytics/conversion-funnel` | Lead stage funnel |
| `GET` | `/analytics/top-keywords` | Top 10 keywords by mention count |

### Notifications

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/notifications/webhooks` | Get webhook config |
| `PUT` | `/notifications/webhooks` | Save Slack/Discord/Email config |
| `POST` | `/notifications/webhooks/test` | Send test notification |

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/healthz` | Liveness check |
| `GET` | `/readyz` | Readiness check (DB + Redis) |

---

## Signal Engine

The monitor runs as a background goroutine alongside the API server. It polls social platforms on a configurable interval (default: 5 minutes).

### How It Works

```
tick()
  ├── Fetch all active keywords (across all workspaces)
  ├── Group by workspace
  └── For each workspace:
      ├── For each keyword:
      │   ├── For each platform in keyword.platforms:
      │   │   ├── HN: Search Algolia API → filter → insert mentions
      │   │   └── Reddit: Fetch /r/{sub}/new.json per subreddit → filter → insert
      │   └── 3s pause between keywords
      └── Send notifications for new mentions
```

### Hacker News

- **API:** Algolia HN Search (`hn.algolia.com/api/v1/search_by_date`)
- **Strategy:** Search for keyword, filter by recency (last 24h), deduplicate by `platform_id`
- **Rate limits:** Generous (no auth required, no observed throttling)

### Reddit

- **API:** Public subreddit JSON feeds (`reddit.com/r/{subreddit}/new.json`)
- **Strategy:** Fetch latest 25 posts per subreddit, filter locally by keyword using `filterContent()`
- **Rate limits:** 2s delay between subreddit requests, 10-minute backoff on 429
- **Requires:** User must specify subreddits per keyword — no blind search (avoids rate limits)

### Content Filtering (`filter.go`)

Every post goes through `filterContent()` before being saved:

1. **Minimum length** — reject posts under 20 characters (spam/empty)
2. **Negative terms** — case-insensitive check against keyword's negative_terms list
3. **Match type:**
   - `broad` — keyword appears anywhere (case-insensitive substring)
   - `exact` — keyword appears as a standalone word (word-boundary matching)
   - `phrase` — exact phrase match (case-insensitive)

### Notifications (`notify.go`)

When new mentions are found, the monitor checks the workspace's webhook config and sends alerts to:

- **Slack** — rich message with mention details + link
- **Discord** — embed with mention details + link
- **Email** — HTML email via Resend API from `LeadEcho <lead@illuminate.sh>`

---

## AI Pipeline

Located in `backend/internal/ai/ai.go`. Uses an OpenAI-compatible chat completions API (works with Zhipu GLM, OpenAI, or any compatible provider).

### Intent Classification

Classifies a mention into:

| Intent | Description |
|--------|-------------|
| `buy_signal` | Actively looking to buy/switch |
| `complaint` | Complaining about competitor |
| `recommendation_ask` | Asking for recommendations |
| `comparison` | Comparing products |
| `general` | General discussion |

Returns: `intent`, `conversion_probability` (0-1), `relevance_score` (0-10), `reasoning`

### Reply Drafting

Generates platform-appropriate replies following these rules:

- Sound like a genuine community member, not a salesperson
- Be helpful first, product mention secondary
- Match platform tone (Reddit: casual, HN: technical)
- 2-4 sentences for Reddit/HN
- Uses knowledge base context when available (RAG)

Returns: `reply` text, `tone` (helpful/empathetic/technical/casual)

### Provider Configuration

The system tries providers in order:
1. **Zhipu GLM** (`GLM_API_KEY`) — model: `glm-4.5-flash` (free tier available)
2. **OpenAI** (`OPENAI_API_KEY`) — model: `gpt-4o-mini` (fallback)

---

## Frontend (Dashboard)

Vite + React 19 with TanStack Router (file-based routing) and TanStack Query for server state.

### Design System

**Neobrutalism** aesthetic:
- 2px borders everywhere
- Hard box shadows (no blur)
- High contrast colors
- CVA (Class Variance Authority) for component variants
- Tailwind CSS v4

### Pages

| Route | Page | Description |
|-------|------|-------------|
| `/inbox` | Mention Inbox | All mentions with status filters, classify + draft reply buttons |
| `/pipeline` | Lead Pipeline | CRM-style pipeline with drag-and-drop stages |
| `/analytics` | Analytics | Charts: mentions/day, platform breakdown, intent distribution, top keywords |
| `/keywords` | Keywords | Add/edit keywords with platform selection, match type, negative terms, subreddits |
| `/knowledge-base` | Knowledge Base | Upload documents for RAG-powered AI replies |
| `/alerts` | Alerts | Configure Slack/Discord/Email notification channels |
| `/workflows` | Workflows | Automation builder (coming soon) |
| `/settings` | Settings | Account configuration |

---

## Makefile Commands

```bash
# === Quick Start ===
make up                  # Start DB + API + Dashboard (backgrounded)
make down                # Stop everything
make logs                # Tail all logs
make logs-api            # Tail API logs only
make logs-web            # Tail frontend logs only

# === Database ===
make db-up               # Start Postgres + Redis (Docker)
make db-down             # Stop Docker services
make db-reset            # Destroy and recreate DB + run migrations + seed
make db-connect          # psql into the database
make redis-connect       # redis-cli

# === Migrations ===
make migrate-up          # Run all pending migrations
make migrate-down        # Roll back last migration
make migrate-status      # Show migration status
make migrate-new name=x  # Create new migration file

# === Backend ===
make api-dev             # Run Go API server (foreground)
make api-build           # Build Go binary (CGO_ENABLED=0)
make api-test            # Run Go tests (-race)
make sqlc                # Regenerate Go code from SQL queries

# === Frontend ===
make web-dev             # Run Vite dev server (foreground)
make web-build           # Production build
make web-preview         # Preview production build
make web-check           # TypeScript type check

# === Utilities ===
make install             # Install all dependencies (Go + Node)
make build               # Production build (frontend + backend)
make check               # Type check + tests
make fmt                 # Format all Go code
make lint                # Lint Go + TypeScript
make clean               # Remove build artifacts
```

---

## Deployment

### Docker Compose (Local)

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    ports: ["5433:5432"]
    environment:
      POSTGRES_USER: leadecho
      POSTGRES_PASSWORD: leadecho
      POSTGRES_DB: leadecho_dev

  redis:
    image: redis:7-alpine
    ports: ["6380:6379"]
```

### Production

The backend builds into a ~15MB distroless container:

```bash
# Backend
cd backend && CGO_ENABLED=0 go build -o bin/leadecho ./cmd/api

# Frontend
cd dashboard && pnpm build
```

**Recommended platforms:** Railway, Fly.io, Render, or any Docker host.

**Required services:**
- PostgreSQL 16+ with pgvector extension
- Redis 7+

---

## Roadmap

### v1 (Current)

- [x] HN + Reddit monitoring
- [x] AI intent classification (GLM / OpenAI)
- [x] AI reply drafting with KB context
- [x] Unified mention inbox
- [x] Lead pipeline
- [x] Analytics dashboard
- [x] Slack/Discord/Email notifications
- [x] Keyword management with subreddit tracking
- [x] Content relevance filtering
- [x] Email + Google OAuth auth

### v2 (Next)

- [ ] Twitter/X monitoring (OAuth2 app credentials)
- [ ] Chrome extension for LinkedIn monitoring + in-platform reply posting
- [ ] Auto-post replies from dashboard
- [ ] Workflow automation (trigger → action chains)
- [ ] UTM link tracking (reply → click → signup)
- [ ] Team seats + RBAC

### v3 (Pro Plan)

- [ ] Multi-workspace
- [ ] BYOK API keys
- [ ] Stripe billing integration
- [ ] Priority crawl frequency
- [ ] Advanced analytics (materialized views)
- [ ] API access for integrations

---

## Contributing

LeadEcho is open source. Contributions welcome.

```bash
# Fork + clone
git clone https://github.com/your-org/leadecho.git

# Setup
make install
cp .env.example .env
make up

# Make changes
make api-test        # Run backend tests
make web-check       # Type check frontend
make lint            # Lint everything
```

---

## License

[MIT](LICENSE)
