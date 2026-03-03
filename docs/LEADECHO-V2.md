# LeadEcho v2 — Architecture & Implementation Plan

**From Keyword Alerts to Intent-Driven Lead Discovery**

---

## The Problem

LeadEcho v1 is a keyword-based social monitoring tool. It works, but has three fundamental limitations:

### 1. Keywords Miss the Best Leads

The highest-value leads never use your keywords. They describe their pain in natural language:

| What the user says | What they need | Keyword match? |
|---|---|---|
| "We're losing deals in spreadsheets" | CRM | None |
| "Tracking customers is falling apart at 50 people" | CRM | None |
| "Salesforce is killing our budget" | CRM alternative | Maybe |
| "Spent 3 hours copying data between tools today" | CRM | None |

These are active buyers. Keyword monitoring misses all of them.

### 2. Server-Side Crawling Gets Shut Down

| Platform | Problem |
|---|---|
| Reddit | Rate-limits unauthenticated scraping. GummySearch (135K users) shut down Dec 2025 after Reddit denied commercial API license |
| Twitter/X | API costs $5K/mo for full access. Most indie tools skip it entirely |
| LinkedIn | No public API. Scraping gets accounts banned |
| All platforms | TOS violations, rate limits, proxy costs, shutdown risk |

Our Reddit crawler already hits 429s constantly. This approach doesn't scale.

### 3. No Automatic Intelligence

Users manually click "Classify" and "Draft Reply" on every mention. At 100+ mentions/day, this is unusable. The 5% that are actually valuable get buried in noise.

---

## The Solution: Three Architectural Shifts

```
V1: Keywords → String Match → Alert → Manual Triage → Maybe Reply

V2: Pain-Point Profiles → Broad Collection → Semantic Match →
    Auto AI Scoring → Scored Inbox → AI Reply Draft → Conversion Tracking
```

### Shift 1: Pain-Point Profiles Replace Keywords

Users describe the problems they solve in natural language. These are embedded as vectors (Voyage AI, 1024-dim). Monitoring uses semantic similarity, not string matching.

Keywords still exist as one signal among many — useful for competitor names and product mentions. But they're no longer the primary mechanism.

### Shift 2: Server-Side Browser Automation Replaces Raw API Crawling

Instead of making unauthenticated API requests that get rate-limited, we use **Pinchtab** and **Camoufox** — server-side browser automation tools that maintain real authenticated sessions. From the platform's perspective, this is indistinguishable from a real user browsing.

### Shift 3: Automatic 4-Stage Scoring Pipeline

Every post is scored automatically on ingestion. Cheap filters run first; expensive LLM calls only run on posts that survive earlier stages. Users see "7 qualified leads" instead of "847 unfiltered mentions."

---

## Data Collection Architecture

### The Three-Tier Collection Model

```
┌─────────────────────────────────────────────────────────────────┐
│                    DATA COLLECTION LAYER                         │
│                                                                  │
│  Tier 1: Direct APIs (always-on, no browser needed)             │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  HN: Algolia API (free, stable, generous)                 │  │
│  │  Dev.to: Public API (dev.to/api/articles)                 │  │
│  │  Lobsters: RSS (lobste.rs/rss)                            │  │
│  │  IndieHackers: RSS (indiehackers.com/feed.xml)            │  │
│  │  ProductHunt: RSS + discussion pages                      │  │
│  │  Stack Overflow: Public API (api.stackexchange.com)       │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Tier 2: Pinchtab — Go browser sidecar (Reddit + Twitter)       │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  12MB Go binary, HTTP/JSON API, persistent Chrome profile │  │
│  │  ┌──────────────┐  ┌──────────────┐                       │  │
│  │  │   Reddit     │  │  Twitter/X   │                       │  │
│  │  │  Authenticated│  │  Search page │                       │  │
│  │  │  /r/sub/new  │  │  with session │                       │  │
│  │  │  + search    │  │  (free, no   │                       │  │
│  │  │  + comments  │  │   API costs) │                       │  │
│  │  └──────────────┘  └──────────────┘                       │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Tier 3: Camoufox — Stealth Firefox (LinkedIn only)             │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Firefox-based, C++ level fingerprint spoofing            │  │
│  │  ~200MB RAM, Python sidecar, WebSocket control            │  │
│  │  ┌──────────────┐                                         │  │
│  │  │  LinkedIn    │  Maximum stealth needed —               │  │
│  │  │  Feed scan   │  LinkedIn's bot detection is            │  │
│  │  │  + search    │  the most aggressive of all platforms   │  │
│  │  └──────────────┘                                         │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Optional: Chrome Extension (supplementary)                      │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Passive DOM scanning while user browses (zero requests)  │  │
│  │  Reply posting from within the platform                   │  │
│  │  NOT the primary collection method                        │  │
│  └────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                    POST /api/v1/mentions/ingest
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                    INTELLIGENCE LAYER (Go Backend)               │
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌────────────────────────┐  │
│  │  4-Stage    │  │  Semantic   │  │  Pain-Point Profile    │  │
│  │  Scoring    │  │  Discovery  │  │  Matching (pgvector)   │  │
│  │  Pipeline   │  │  Engine     │  │                        │  │
│  └──────┬──────┘  └──────┬──────┘  └────────────┬───────────┘  │
│         │                │                       │              │
│  ┌──────▼────────────────▼───────────────────────▼───────────┐  │
│  │                   PostgreSQL + pgvector                    │  │
│  └──────────────────────────┬────────────────────────────────┘  │
│                             │                                    │
│  ┌──────────────────────────▼────────────────────────────────┐  │
│  │     Smart Inbox (Leads Ready / Worth Watching / Filtered) │  │
│  └───────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Why Pinchtab + Camoufox (Not Just Chrome Extension)

The V2 spec proposed a "browser-first" approach using a Chrome extension as the primary data collector. After analysis, a **server-side browser automation** approach is superior:

| Dimension | Chrome Extension | Pinchtab + Camoufox |
|---|---|---|
| **Uptime** | Only when user's laptop is open | 24/7 server-side |
| **Reliability** | MV3 kills service workers after 30s idle | Persistent process, always running |
| **Multi-device** | Per-device, fragmented | Centralized, one instance per user |
| **Go integration** | Message passing, complex | HTTP API (Pinchtab) / WebSocket (Camoufox) |
| **Deployment** | Chrome Web Store review process | Docker sidecar, instant |
| **Debugging** | Remote, on user's machine | Server-side, full observability |

The Chrome extension becomes a **supplementary** tool for:
- Passive DOM scanning while browsing (free extra data)
- In-platform reply posting (user engagement)
- NOT the primary monitoring method

### Why Two Tools?

**Pinchtab** (Go, 12MB, Chrome-based):
- Native Go binary — fits our stack perfectly
- HTTP/JSON API — controllable with plain `net/http`
- Persistent Chrome profiles — login once, sessions survive restarts
- Multi-instance management built-in (named profiles, separate ports)
- `humanClick` / `humanType` for realistic behavior
- `BRIDGE_STEALTH=full` for canvas/WebGL/font spoofing
- Good enough for Reddit + Twitter (moderate anti-bot)

**Camoufox** (Python, ~200MB, Firefox-based):
- C++ level fingerprint spoofing — invisible to JavaScript inspection
- Firefox engine — less targeted by anti-bot systems (they focus on Chrome)
- WebGL renderer spoofing at the rendering engine level
- WebRTC IP leak protection at protocol level
- Controllable from Go via WebSocket remote server + playwright-go
- Required for LinkedIn — the most aggressive anti-bot platform

```
Reddit    → Pinchtab   (moderate stealth needed, Chrome fine)
Twitter/X → Pinchtab   (moderate stealth, session-based fetch)
LinkedIn  → Camoufox   (maximum stealth, Firefox + C++ spoofing)
HN        → Algolia    (no browser needed, free public API)
Dev.to    → Public API (no browser needed)
RSS feeds → Direct     (no browser needed)
```

---

## Pinchtab Integration Design

### Deployment

```yaml
# docker-compose.yml addition
services:
  pinchtab:
    image: pinchtab/pinchtab:latest
    ports:
      - "127.0.0.1:9867:9867"
    environment:
      BRIDGE_TOKEN: ${PINCHTAB_TOKEN}
      BRIDGE_HEADLESS: true
      BRIDGE_STEALTH: full
      BRIDGE_BLOCK_IMAGES: true
      BRIDGE_BLOCK_MEDIA: true
      BRIDGE_MAX_TABS: 5
    volumes:
      - pinchtab_profiles:/root/.pinchtab
    mem_limit: 2g
    cpus: 2.0
```

### How the Go Backend Controls Pinchtab

Pinchtab exposes a clean HTTP/JSON API. No SDK, no bindings — just HTTP:

```go
// backend/internal/browser/pinchtab.go

type PinchtabClient struct {
    baseURL string
    token   string
    http    *http.Client
}

func (p *PinchtabClient) Navigate(url string) error {
    return p.post("/navigate", map[string]string{"url": url})
}

func (p *PinchtabClient) GetText() (string, error) {
    return p.get("/text")  // Returns readable text, ~800 tokens/page
}

func (p *PinchtabClient) InjectCookies(cookies []Cookie) error {
    return p.post("/cookies", cookies)
}

func (p *PinchtabClient) EvaluateJS(script string) (string, error) {
    return p.post("/evaluate", map[string]string{"expression": script})
}
```

### Reddit via Pinchtab

```
1. User provides Reddit session cookie (one-time setup in dashboard)
2. Backend injects cookie into Pinchtab: POST /cookies
3. Every 5 min, for each monitored subreddit:
   a. Navigate to reddit.com/r/{sub}/new.json (authenticated)
   b. GET /text → parse JSON response
   c. Apply local filters (negative terms, match type)
   d. Insert matching posts via scoring pipeline
4. Authenticated requests get 60 req/min (vs constant 429s)
```

### Twitter/X via Pinchtab

```
1. User provides X session cookie (one-time setup)
2. Backend injects cookie into Pinchtab: POST /cookies
3. Every 5 min, for each pain-point profile:
   a. Navigate to x.com/search?q={query}&f=live
   b. GET /text → extract tweet content
   c. Parse tweets (author, content, engagement)
   d. Insert via scoring pipeline
4. Free — no $5K/mo API cost
```

---

## Camoufox Integration Design

### Deployment

```yaml
# docker-compose.yml addition
services:
  camoufox:
    build:
      context: ./camoufox-sidecar
    ports:
      - "127.0.0.1:1234:1234"
    environment:
      CAMOUFOX_PORT: 1234
      DISPLAY: ":99"  # Virtual display for headful stealth
    volumes:
      - camoufox_data:/app/data
    mem_limit: 512m
    profiles: ["linkedin"]  # Only starts when LinkedIn monitoring enabled
```

### Camoufox Sidecar (Python)

```python
# camoufox-sidecar/server.py
from camoufox.sync_api import Camoufox

# Launches stealth Firefox with fingerprint rotation
# Exposes WebSocket for Go backend to connect via playwright-go
camoufox = Camoufox(
    headless=False,  # Virtual display mode (stealthier)
    humanize=True,   # Human-like mouse movements
)
server = camoufox.launch_server(port=1234, ws_path="/browser")
```

### Go Backend Connects via playwright-go

```go
// backend/internal/browser/camoufox.go

func (c *CamoufoxClient) ScanLinkedIn(query string) ([]RawPost, error) {
    browser, _ := pw.Firefox.Connect("ws://camoufox:1234/browser")
    page, _ := browser.NewPage()

    // Navigate with human-like delays
    page.Goto("https://www.linkedin.com/search/results/content/?keywords=" + query)
    time.Sleep(randomDelay(2, 5))  // Human-like wait

    // Extract posts from DOM
    posts := page.QuerySelectorAll("div.feed-shared-update-v2")
    // ... parse each post

    return rawPosts, nil
}
```

### LinkedIn Constraints

- Max 5 searches per hour (randomized intervals)
- Only for users who explicitly enable LinkedIn monitoring
- Pro plan only (resource-intensive)
- Rate limited at the application level to protect user's LinkedIn account

---

## Pain-Point Profiles

### Concept

A Pain-Point Profile replaces the keyword as the core monitoring configuration. Instead of "CRM alternative", the user describes:

```
Product: AcmeCRM
Description: Simple CRM for small B2B teams who've outgrown spreadsheets

Pain points:
1. "Teams losing track of customer conversations across email, Slack, phone"
2. "Spending hours on manual data entry between spreadsheets and sales tools"
3. "No visibility into sales pipeline, deals falling through cracks"
4. "Small teams outgrowing spreadsheet-based customer tracking"

Competitors: Salesforce, HubSpot, Pipedrive, Close

Ideal customer: 10-100 employees, B2B SaaS / agencies

Platforms: Reddit, HN, Twitter, LinkedIn
Subreddits: r/smallbusiness, r/SaaS, r/startups, r/sales
```

### How It Works

On save, each pain-point description is embedded using Voyage AI (voyage-3, 1024 dimensions). When a new post arrives:

1. Post content is embedded (same model)
2. Cosine similarity computed against all pain-point embeddings (pgvector)
3. Score > 0.4 → relevant, proceeds to AI scoring
4. Score 0.3-0.4 → "Worth Watching" tier
5. Score < 0.3 → auto-archived

### Database

```sql
CREATE TABLE monitoring_profiles (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id            UUID NOT NULL REFERENCES workspaces(id),
    name                    TEXT NOT NULL,
    product_name            TEXT NOT NULL,
    product_description     TEXT NOT NULL,
    pain_points             TEXT[] NOT NULL DEFAULT '{}',
    competitors             TEXT[] DEFAULT '{}',
    keywords                TEXT[] DEFAULT '{}',       -- legacy keyword support
    negative_terms          TEXT[] DEFAULT '{}',
    ideal_customer          TEXT,
    company_size_range      TEXT,
    industries              TEXT[] DEFAULT '{}',
    platforms               platform_type[] DEFAULT '{reddit,hackernews}',
    subreddits              TEXT[] DEFAULT '{}',
    communities             JSONB DEFAULT '{}',         -- dev.to tags, etc.
    min_relevance_score     REAL DEFAULT 0.4,
    is_active               BOOLEAN DEFAULT true,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW()
);

-- Pain-point embeddings stored separately for efficient vector queries
CREATE TABLE pain_point_embeddings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id      UUID NOT NULL REFERENCES monitoring_profiles(id) ON DELETE CASCADE,
    pain_point_text TEXT NOT NULL,
    embedding       vector(1024) NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_pain_point_embeddings
    ON pain_point_embeddings USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Mentions: add embedding + scoring columns
ALTER TABLE mentions ADD COLUMN content_embedding   vector(1024);
ALTER TABLE mentions ADD COLUMN matched_profile_id  UUID REFERENCES monitoring_profiles(id);
ALTER TABLE mentions ADD COLUMN matched_pain_point  TEXT;
ALTER TABLE mentions ADD COLUMN intent_reasoning    TEXT;
ALTER TABLE mentions ADD COLUMN urgency             TEXT;        -- low/medium/high
ALTER TABLE mentions ADD COLUMN icp_match           BOOLEAN;
ALTER TABLE mentions ADD COLUMN lead_score          INTEGER;     -- 0-100
ALTER TABLE mentions ADD COLUMN source              TEXT DEFAULT 'server';
ALTER TABLE mentions ADD COLUMN inbox_tier          TEXT DEFAULT 'filtered';

CREATE INDEX idx_mentions_embedding ON mentions
    USING hnsw (content_embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX idx_mentions_inbox
    ON mentions (workspace_id, inbox_tier, created_at DESC);
```

---

## 4-Stage AI Scoring Pipeline

Every collected post passes through this pipeline automatically on ingestion:

```
Post arrives (from any source: Algolia, Pinchtab, Camoufox, RSS)
│
├─ Stage 1: Rule-Based Noise Filter                    Cost: $0
│   • Min content length (< 30 chars → reject)
│   • Self-promo detection ("I built", "check out my", "just launched")
│   • Job listing detection ("hiring", "salary range", "apply now")
│   • Bot/mod detection (AutoModerator, "I am a bot")
│   • Negative terms from profile
│   • Expected: ~60% filtered
│
├─ Stage 2: Semantic Relevance Scoring                 Cost: ~$0.001/post
│   • Embed post content (Voyage AI)
│   • Compare against all pain-point embeddings (pgvector cosine similarity)
│   • > 0.4 → Stage 3
│   • 0.3-0.4 → "Worth Watching" (skip Stage 3-4)
│   • < 0.3 → auto-archived
│   • Competitor name mention → boost to min 0.6
│   • Expected: ~70% of remaining filtered
│
├─ Stage 3: Intent Classification                      Cost: ~$0.003/post
│   • LLM (Claude Haiku or GLM) classifies intent
│   • buy_signal → Leads Ready + Stage 4
│   • recommendation_ask → Leads Ready + Stage 4
│   • pain_point → Worth Watching
│   • complaint → depends on score
│   • comparison → Worth Watching
│   • general → Filtered
│   • Also outputs: conversion_probability, urgency, reasoning
│   • Runs on: ~12% of original posts
│
└─ Stage 4: Lead Qualification + Reply Drafting        Cost: ~$0.01/post
    • Smarter model (Claude Sonnet or GLM-4.5)
    • Three reply variants:
    │   • value_only: Pure help, no product mention
    │   • soft_mention: Helpful with subtle product reference
    │   • direct: Clear product recommendation (strong intent only)
    • ICP match assessment
    • Lead score (0-100)
    • Runs on: ~3-5% of original posts
```

### Cost Per User

| Daily volume | Stage 1 (free) | Stage 2 (embed) | Stage 3 (LLM) | Stage 4 (LLM) | Total/day |
|---|---|---|---|---|---|
| 500 posts | → 200 | → 60 | $0.18 | $0.15 | ~$0.43 |
| 1,000 posts | → 400 | → 120 | $0.36 | $0.40 | ~$0.88 |
| 5,000 posts | → 2,000 | → 600 | $1.80 | $1.50 | ~$4.00 |

Monthly cost at 1K posts/day: ~$25-30/month in AI costs.

---

## Smart Inbox (3 Tiers)

Replace the current flat mention list with a scored, tiered inbox:

### Tier 1: Leads Ready

Posts classified as `buy_signal` or `recommendation_ask` with > 0.6 conversion probability. Each has pre-generated reply drafts (3 variants). Badge count in sidebar.

### Tier 2: Worth Watching

Posts classified as `pain_point` or `comparison` with moderate relevance (0.3-0.6). Not ready to engage but represent emerging opportunities. One-click promote to Leads Ready.

### Tier 3: Filtered (Auto-Archived)

Everything that didn't make the cut. Accessible but hidden by default. Shows aggregate counts by filter reason for profile tuning.

### Inbox Metrics Bar

```
Today: 12 leads ready • 47 worth watching • 834 filtered
This week: 67 leads • 23 replies sent • 4 converted
Top signal: "sales pipeline" pain point (38% of leads)
```

---

## Semantic Discovery Engine

A batch process that finds leads no keyword could catch. Runs nightly (or hourly for Pro):

```sql
SELECT m.*, 1 - (m.content_embedding <=> pp.embedding) as similarity
FROM mentions m
CROSS JOIN pain_point_embeddings pp
WHERE m.created_at > NOW() - INTERVAL '24 hours'
  AND m.intent IS NULL
  AND 1 - (m.content_embedding <=> pp.embedding) > 0.45
ORDER BY similarity DESC
LIMIT 200;
```

Discovered posts get a "Discovered" badge in inbox. These are the leads no competitor can find.

---

## Platform Strategy Summary

| Platform | Collection | Auth | Difficulty | Priority |
|---|---|---|---|---|
| **Hacker News** | Server: Algolia API | None needed | Easy | P0 (done) |
| **Reddit** | Pinchtab: /r/{sub}/new.json | Session cookie | Easy | P0 |
| **Dev.to** | Server: Public API | None needed | Easy | P0 |
| **Lobsters** | Server: RSS | None needed | Easy | P0 |
| **IndieHackers** | Server: RSS | None needed | Easy | P0 |
| **Twitter/X** | Pinchtab: search page | Session cookie | Medium | P1 |
| **LinkedIn** | Camoufox: search + feed | Session cookie | Hard | P1 |
| **ProductHunt** | Server: RSS + pages | None needed | Easy | P2 |
| **Stack Overflow** | Server: Public API | None needed | Easy | P2 |

---

## Build Roadmap

### Phase 1: Pain-Point Profiles + Auto-Scoring (Weeks 1-3)

The biggest differentiator. Works with existing server-side crawlers — no browser changes needed.

- `monitoring_profiles` table + CRUD API + embedding on save
- `pain_point_embeddings` table with pgvector index
- `mentions.content_embedding` column
- 4-stage scoring pipeline (async worker pool, Redis-backed job queue)
- Smart inbox API (3 tiers based on scoring)
- Dashboard: profile setup wizard + inbox redesign
- SSE for real-time inbox push

### Phase 2: More Server-Side Platforms (Weeks 3-4)

Easy wins — all public APIs/RSS, no auth needed:

- Dev.to adapter (public API: `dev.to/api/articles?tag={tag}`)
- Lobsters adapter (RSS: `lobste.rs/rss`)
- IndieHackers adapter (RSS: `indiehackers.com/feed.xml`)
- Semantic Discovery Engine (nightly batch job)
- Reply variants (3 per high-intent mention)

### Phase 3: Pinchtab Integration — Reddit + Twitter (Weeks 4-6)

Server-side browser automation for platforms that need auth:

- Pinchtab Docker sidecar setup
- `browser/pinchtab.go` — Go HTTP client for Pinchtab API
- Reddit fetcher: authenticated `/r/{sub}/new.json` + comment monitoring
- Twitter/X fetcher: search page with session cookie
- Session cookie management UI in dashboard (one-time setup per platform)
- Heartbeat + fallback logic (if Pinchtab offline, use existing server crawlers)
- Replace current unauthenticated Reddit crawler

### Phase 4: Camoufox Integration — LinkedIn (Weeks 6-8)

Heavy stealth for the hardest platform:

- Camoufox Python sidecar (Docker, virtual display mode)
- `browser/camoufox.go` — Go client connecting via WebSocket + playwright-go
- LinkedIn feed scanning (passive, max 5 searches/hour)
- LinkedIn content parsing
- Rate limiter at application level (protect user's LinkedIn account)
- Pro plan only (resource-intensive)

### Phase 5: Chrome Extension (Weeks 8-10)

Supplementary, not primary:

- MV3 scaffold, service worker, popup
- Passive content scripts (Reddit, Twitter, LinkedIn, HN)
- Scans visible posts during normal browsing — zero extra requests
- Sends discovered posts to `POST /api/v1/mentions/ingest`
- In-platform reply posting (user clicks "Post" in dashboard → extension posts)

### Phase 6: Polish & Launch (Weeks 10-12)

- Onboarding wizard (profile setup, subreddit recommendations)
- UTM link generation + click tracking + conversion attribution
- Chrome Web Store submission
- Landing page + docs site
- ProductHunt launch

---

## What We Keep From V1

- HN Algolia crawler (works perfectly, no change)
- Reddit subreddit monitoring as server-side fallback
- Existing database schema (additive migrations only)
- Existing auth (email + Google OAuth)
- Existing notifications (Slack, Discord, Resend email)
- Existing lead pipeline
- Existing knowledge base (documents table becomes source for reply context)
- Neobrutalism design system

---

## What Changes From V1

| V1 | V2 |
|---|---|
| Keywords as primary primitive | Pain-Point Profiles (keywords become one signal) |
| Flat mention inbox | 3-tier scored inbox (Leads Ready / Watching / Filtered) |
| Manual "Classify" button | Auto-classification on ingestion |
| Manual "Draft Reply" button | Auto-drafted reply variants for high-intent |
| Unauthenticated Reddit scraping (429s) | Pinchtab with authenticated sessions |
| No Twitter/LinkedIn support | Pinchtab (Twitter) + Camoufox (LinkedIn) |
| HN + Reddit only | 9 platforms (HN, Reddit, Twitter, LinkedIn, Dev.to, Lobsters, IH, PH, SO) |
| No semantic matching | pgvector cosine similarity against pain-point embeddings |
| No discovery | Semantic Discovery Engine finds leads keywords miss |

---

## Cost Model

### Per-User Infrastructure (Hosted)

| Component | Monthly Cost | Notes |
|---|---|---|
| AI scoring (1K posts/day) | $25-30 | Haiku classify + Voyage embed |
| Embeddings (Voyage AI) | $5-10 | Pain-point + post embeddings |
| Pinchtab (Reddit + Twitter) | $5-10 | Shared instance, Chrome ~1.5GB |
| Camoufox (LinkedIn, optional) | $10-15 | ~200MB, Pro plan only |
| PostgreSQL + pgvector | $10-20 | Small instance |
| Redis | $5-10 | Job queue + cache |
| Go server | $10-20 | Single binary |
| **Total per user** | **$70-115** | |

### Pricing

| Tier | Price | Includes |
|---|---|---|
| Free (self-hosted) | $0 | Everything, BYOK for AI + browser |
| Starter (hosted) | $29/mo | 500 posts/day, 1 profile, HN + Reddit + dev communities |
| Pro (hosted) | $79/mo | 2K posts/day, 5 profiles, all platforms incl. LinkedIn, discovery |
| Team (hosted) | $149/mo | 5K posts/day, unlimited profiles, team seats, priority support |

---

## Competitive Positioning

```
"Describe your problem, we'll find your buyers."

LeadEcho is the open-source social intent monitor that finds people
experiencing the problems your product solves — across Reddit, HN,
Twitter, LinkedIn, and developer communities — scores their buying
intent, and drafts your reply.
```

| Capability | F5Bot | Syften | KWatch | Octolens | Brand24 | **LeadEcho v2** |
|---|---|---|---|---|---|---|
| Price | Free | $29 | $19 | ~$49 | $79-399 | **Free (OSS)** |
| Self-hostable | No | No | No | No | No | **Yes** |
| Semantic matching | No | No | No | No | No | **Yes** |
| Pain-point profiles | No | No | No | No | No | **Yes** |
| Auto-classification | No | No | No | Partial | Partial | **4-stage** |
| Reply drafting | No | No | No | No | No | **3 variants** |
| Reddit | Yes | Yes | Yes | Yes | Yes | **Yes** |
| Twitter/X | No | Yes | Yes | Ltd | Yes | **Yes** |
| LinkedIn | No | No | No | No | Ltd | **Yes** |
| Can't be shut down | No | No | No | No | No | **Yes (OSS + browser automation)** |

---

*This is a living document. Updated as implementation progresses.*
