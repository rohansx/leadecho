# LeadEcho - System Architecture

## Architecture Overview

LeadEcho uses a **Hybrid Cloud + Browser Extension** architecture. The cloud handles 24/7 monitoring and AI processing, while the Chrome Extension handles platform engagement using the user's authenticated sessions.

```
                        ┌─────────────────────────────────────────────────────────────┐
                        │                    CLOUD (Railway)                           │
                        │                                                             │
                        │  ┌────────────────────────────────────────────────────────┐  │
                        │  │                   Go API Gateway                       │  │
                        │  │              (Chi Router + Middleware)                  │  │
                        │  │      Auth │ CORS │ Rate Limit │ Logging │ SSE         │  │
                        │  └─────┬──────────┬──────────────┬────────────────────────┘  │
                        │        │          │              │                           │
                        │  ┌─────▼────┐ ┌───▼────────┐ ┌──▼─────────────┐            │
                        │  │  Signal   │ │  RAG Brain │ │  Workflow      │            │
                        │  │  Engine   │ │            │ │  Engine        │            │
                        │  │          │ │  Claude API │ │                │            │
                        │  │ HN Poller│ │  pgvector   │ │ Task Queue    │            │
                        │  │ Reddit   │ │  Embeddings │ │ Approval Gates│            │
                        │  │ X/Twitter│ │  Persona    │ │ Slack/Discord │            │
                        │  │          │ │  Matching   │ │ Webhooks      │            │
                        │  └─────┬────┘ └──────┬─────┘ └──────┬────────┘            │
                        │        │             │              │                       │
                        │  ┌─────▼─────────────▼──────────────▼────────────────────┐  │
                        │  │                    Redis                               │  │
                        │  │   Pub/Sub │ Task Queue │ Rate Limiting │ Cache         │  │
                        │  └───────────────────────┬───────────────────────────────┘  │
                        │                          │                                  │
                        │  ┌───────────────────────▼───────────────────────────────┐  │
                        │  │                  PostgreSQL                            │  │
                        │  │   Users │ Mentions │ Leads │ Analytics │ pgvector     │  │
                        │  │   Materialized Views │ UTM Tracking                   │  │
                        │  └───────────────────────┬───────────────────────────────┘  │
                        │                          │                                  │
                        └──────────────────────────┼──────────────────────────────────┘
                                                   │
                                          SSE / REST API
                                                   │
                        ┌──────────────────────────┼──────────────────────────────────┐
                        │                   CLIENT LAYER                               │
                        │                          │                                   │
                        │  ┌───────────────────────▼───────────────────────────────┐   │
                        │  │              Next.js Dashboard                         │   │
                        │  │                                                       │   │
                        │  │  ┌───────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐  │   │
                        │  │  │  Intent   │ │  Lead    │ │ Analytics│ │  RAG   │  │   │
                        │  │  │  Inbox    │ │ Pipeline │ │ Dashboard│ │ Editor │  │   │
                        │  │  └───────────┘ └──────────┘ └──────────┘ └────────┘  │   │
                        │  │  ┌───────────┐ ┌──────────┐ ┌──────────────────────┐  │   │
                        │  │  │  Team     │ │ Settings │ │ Workflow Builder     │  │   │
                        │  │  │  Mgmt     │ │          │ │                      │  │   │
                        │  │  └───────────┘ └──────────┘ └──────────────────────┘  │   │
                        │  └───────────────────────────────────────────────────────┘   │
                        │                                                              │
                        │  ┌───────────────────────────────────────────────────────┐   │
                        │  │          Chrome Extension (Manifest V3)                │   │
                        │  │                                                       │   │
                        │  │  ┌──────────┐ ┌────────────┐ ┌────────────────────┐  │   │
                        │  │  │ LinkedIn │ │ Cross-     │ │ Human-Mimicry      │  │   │
                        │  │  │ Monitor  │ │ Platform   │ │ Engine             │  │   │
                        │  │  │ (Feed)   │ │ Reply UI   │ │ (Typing/Delays)    │  │   │
                        │  │  └──────────┘ └────────────┘ └────────────────────┘  │   │
                        │  └───────────────────────────────────────────────────────┘   │
                        └──────────────────────────────────────────────────────────────┘
```

---

## Core Design Principles

### 1. Platform Adapter Pattern
Each social platform is implemented as an independent adapter behind a common interface. This enables:
- Adding new platforms without touching core logic
- Swapping API implementations (e.g., Reddit API vs `.json` fallback)
- Independent testing and rate limiting per platform
- Platform-specific configuration without leaking into other modules

### 2. Event-Driven Architecture
All components communicate through Redis pub/sub events:
- Signal Engine publishes `mention.detected` events
- RAG Brain subscribes and publishes `reply.drafted` events
- Workflow Engine orchestrates the approval pipeline
- Dashboard receives real-time updates via SSE

### 3. Human-in-the-Loop by Default
No automated posting without explicit human approval. The system defaults to "suggest and wait" rather than "detect and post."

### 4. Safety-First Engagement
Every engagement action passes through the Safe Link rules engine before reaching the user, ensuring account safety and platform compliance.

---

## Component Architecture

### Signal Engine (Go)

```
signal-engine/
├── platforms/
│   ├── adapter.go          # Platform interface definition
│   ├── hackernews/
│   │   ├── poller.go       # Firebase API + Algolia search polling
│   │   ├── parser.go       # HN data normalization
│   │   └── config.go       # HN-specific settings
│   ├── reddit/
│   │   ├── poller.go       # OAuth API + .json fallback
│   │   ├── parser.go       # Reddit data normalization
│   │   ├── auth.go         # OAuth token management
│   │   └── config.go       # Reddit-specific settings
│   ├── twitter/
│   │   ├── poller.go       # API v2 Basic tier
│   │   ├── scraper.go      # Playwright fallback
│   │   ├── parser.go       # X data normalization
│   │   └── config.go       # X-specific settings
│   └── linkedin/
│       ├── receiver.go     # Receives signals from Chrome extension
│       └── parser.go       # LinkedIn data normalization
├── scoring/
│   ├── relevance.go        # AI relevance scoring (1-10)
│   ├── intent.go           # Intent classification
│   └── priority.go         # Conversion probability scoring
├── dedup/
│   └── dedup.go            # Deduplication across platforms
└── engine.go               # Orchestrator: starts/stops pollers
```

### Browser Sidecar Layer

The Signal Engine delegates authenticated platform crawling to **browser sidecars** — lightweight HTTP microservices wrapping headless browsers. Each sidecar exposes a uniform API (`/navigate`, `/cookies`, `/evaluate`, `/text`, `/health`) so the Go backend swaps between them seamlessly.

```
┌─────────────────────────────────────────────────────────────┐
│                    Monitor Dispatch                          │
│                                                             │
│  Reddit:   Pinchtab → Scrapling → public .json API          │
│  Twitter:  Pinchtab → Scrapling                             │
│  LinkedIn: Camoufox → Pinchtab → Scrapling                  │
│  HN/DevTo: Direct API (no sidecar)                          │
└────────┬──────────────┬──────────────┬──────────────────────┘
         │              │              │
   ┌─────▼─────┐  ┌────▼─────┐  ┌────▼──────┐
   │ Pinchtab  │  │ Camoufox │  │ Scrapling  │
   │ :9867     │  │ :9868    │  │ :9869      │
   │ Chromium  │  │ Firefox  │  │ Chromium   │
   │ stealth   │  │ C++ anti │  │ anti-bot   │
   │           │  │ fingerp. │  │ bypass     │
   └───────────┘  └──────────┘  └────────────┘
```

- **Pinchtab**: Primary sidecar for Reddit and Twitter. Persistent Chromium with full stealth mode.
- **Camoufox**: Pro-tier sidecar for LinkedIn. Firefox with C++ fingerprint spoofing (WebGL, WebRTC, canvas).
- **Scrapling**: Fallback sidecar using D4Vinci's Scrapling framework. StealthyFetcher with adaptive anti-bot bypass.

All sidecars are **optional** (nil-checked). The system degrades gracefully — if no sidecar is configured for a platform, it falls back to public APIs or skips that platform. Session cookies are AES-encrypted in the `platform_accounts` table and injected into sidecars at crawl time.

See [14-browser-sidecar-architecture.md](./14-browser-sidecar-architecture.md) for full implementation details, Docker deployment, and troubleshooting.

### RAG Brain (Go + Claude API)

```
rag-brain/
├── ingestion/
│   ├── processor.go        # Document processing pipeline
│   ├── chunker.go          # Intelligent text chunking
│   ├── embedder.go         # Embedding generation (Voyage/OpenAI)
│   └── formats/
│       ├── markdown.go     # .md file parser
│       ├── pdf.go          # PDF text extraction
│       └── web.go          # URL content scraper
├── retrieval/
│   ├── search.go           # Hybrid search (BM25 + vector)
│   ├── reranker.go         # Result re-ranking
│   └── context.go          # Context window assembly
├── generation/
│   ├── drafter.go          # Reply generation via Claude
│   ├── persona.go          # Persona/voice matching
│   ├── variants.go         # A/B variant generation
│   └── prompts/
│       ├── scoring.go      # Relevance scoring prompts
│       ├── reply.go        # Reply generation prompts
│       └── analysis.go     # Thread analysis prompts
└── learning/
    ├── feedback.go         # Tracks which replies convert
    └── optimizer.go        # Adjusts generation based on outcomes
```

### Workflow Engine (Go + Redis)

```
workflow-engine/
├── engine.go               # Core workflow orchestrator
├── triggers/
│   ├── keyword.go          # Keyword match triggers
│   ├── score.go            # Score threshold triggers
│   └── schedule.go         # Time-based triggers
├── actions/
│   ├── draft.go            # Generate AI draft
│   ├── notify.go           # Send Slack/Discord/email
│   ├── approve.go          # Wait for human approval
│   ├── post.go             # Queue for posting
│   └── track.go            # UTM generation + tracking
├── queue/
│   ├── task.go             # Task definition and management
│   ├── worker.go           # Worker pool for processing
│   └── retry.go            # Retry with exponential backoff
└── integrations/
    ├── slack.go            # Slack webhook + interactive buttons
    ├── discord.go          # Discord webhook integration
    └── email.go            # Email notification (Resend/SES)
```

---

## Data Flow Diagrams

### Flow 1: Mention Detection to Inbox

```
HN Firebase API ──┐
                   │
Reddit API ────────┤   ┌──────────┐   ┌──────────┐   ┌──────────┐
                   ├──►│ Signal   │──►│ AI Score │──►│ Redis    │
X API v2 ──────────┤   │ Engine   │   │ Pipeline │   │ Pub/Sub  │
                   │   └──────────┘   └──────────┘   └────┬─────┘
LinkedIn (ext) ────┘                                       │
                                                           ▼
                                                    ┌──────────┐
                    ┌──────────┐    SSE             │PostgreSQL│
                    │Dashboard │◄───────────────────│ mentions │
                    │ Inbox    │                     │ table    │
                    └──────────┘                     └──────────┘
```

### Flow 2: Reply Generation to Posting

```
User clicks "Draft Reply"
         │
         ▼
┌─────────────────┐     ┌──────────────┐     ┌──────────────┐
│ Thread Context  │────►│  RAG Brain   │────►│ Safe Link    │
│ Full thread     │     │  Claude API  │     │ Rules Engine │
│ ingestion       │     │  + pgvector  │     │              │
└─────────────────┘     └──────────────┘     └──────┬───────┘
                                                     │
                         ┌──────────────┐            │
                         │ 3 Variants   │◄───────────┘
                         │ a) Value     │
                         │ b) Technical │
                         │ c) Soft-sell │
                         └──────┬───────┘
                                │
                    User selects + edits
                                │
                                ▼
                    ┌──────────────────┐
                    │ Approval Gate    │
                    │ (Dashboard/Slack)│
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
         Reddit API    X API v2      Chrome Ext
         (server)      (server)      (LinkedIn)
              │              │              │
              └──────────────┼──────────────┘
                             │
                     UTM Tracking
                     Click → Signup → Revenue
```

### Flow 3: Workflow Automation

```
                    ┌──────────────────────┐
                    │ Trigger Conditions   │
                    │ keyword + platform + │
                    │ score > threshold    │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │ AI Draft Generation  │
                    │ RAG + persona match  │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │ Approval Gate        │
                    │ Slack button / email │
                    │ Dashboard queue      │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │ Human Reviews        │
                    │ ✅ Approve           │
                    │ ✏️  Edit + Approve   │
                    │ ❌ Reject            │
                    └──────────┬───────────┘
                               │ (if approved)
                    ┌──────────▼───────────┐
                    │ Post Queue           │
                    │ Human-mimicry delays │
                    │ Platform-specific    │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │ Track & Learn        │
                    │ UTM → clicks →       │
                    │ signups → revenue    │
                    └──────────────────────┘
```

---

## Communication Patterns

### Internal: Redis Pub/Sub Channels

| Channel | Publisher | Subscriber | Payload |
|---------|-----------|------------|---------|
| `mention.detected` | Signal Engine | RAG Brain, Dashboard (via SSE) | Mention with metadata |
| `mention.scored` | AI Scoring | Workflow Engine, Dashboard | Mention + relevance score |
| `reply.drafted` | RAG Brain | Dashboard, Approval Queue | Draft variants |
| `reply.approved` | Workflow Engine | Post Queue | Approved reply + target |
| `reply.posted` | Post Queue | Analytics, Dashboard | Posted reply + UTM |
| `lead.updated` | Various | Dashboard, CRM Sync | Lead stage change |
| `analytics.event` | Click Tracker | Analytics Engine | UTM click/signup event |

### External: REST API + SSE

| Endpoint Pattern | Method | Purpose |
|-----------------|--------|---------|
| `/api/v1/mentions` | GET | List mentions with filters |
| `/api/v1/mentions/stream` | GET (SSE) | Real-time mention stream |
| `/api/v1/mentions/:id/reply` | POST | Submit reply for posting |
| `/api/v1/leads` | GET/POST/PATCH | Lead pipeline CRUD |
| `/api/v1/analytics/*` | GET | Analytics queries |
| `/api/v1/knowledge-base/*` | GET/POST/DELETE | RAG document management |
| `/api/v1/workflows/*` | GET/POST/PATCH | Workflow CRUD |
| `/api/v1/extension/sync` | POST | Chrome extension data sync |

### Chrome Extension ↔ Cloud

The extension communicates with the cloud backend via:
1. **REST API calls** for fetching mentions, submitting replies, syncing data
2. **WebSocket/SSE** for receiving real-time notifications
3. **Message passing** (chrome.runtime) between content scripts, service worker, and sidepanel

---

## Scaling Considerations

### Phase 1-2 (MVP → V2): Single Railway Service

All Go components run as goroutines within a single binary:
- Signal Engine goroutines per platform
- API Gateway on main goroutine
- Workflow Engine processing queue
- Single PostgreSQL + Redis instance

This is sufficient for up to ~1,000 users and ~50K mentions/month.

### Phase 3+: Service Separation

When scaling beyond ~1,000 users:
- Split Signal Engine into independent service (CPU-bound AI scoring)
- Separate Workflow Engine (needs its own scaling for queue depth)
- Add read replicas for PostgreSQL
- Redis Cluster for pub/sub at scale
- CDN (Cloudflare) for dashboard static assets

### Performance Targets

| Metric | Target | How |
|--------|--------|-----|
| Mention detection latency | < 60s from post | Polling intervals: HN 30s, Reddit 60s, X 60s |
| AI scoring latency | < 3s per mention | Claude Haiku for scoring, Sonnet for drafts |
| Dashboard load time | < 2s | SSR + edge caching + pagination |
| SSE event delivery | < 500ms | Redis pub/sub → SSE bridge |
| Reply posting | 2-8s (intentional) | Human-mimicry delays |
