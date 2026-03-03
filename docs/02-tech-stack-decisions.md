# LeadEcho - Tech Stack Decisions

## Summary Table

| Layer | Technology | Version | Alternative Considered | Why This Choice |
|-------|-----------|---------|----------------------|-----------------|
| **Backend Language** | Go | 1.23+ | Node.js, Python | Goroutines for concurrent polling, strong performance, existing expertise |
| **API Router** | Chi | v5 | Fiber, Echo, Gin | stdlib `net/http` compatible, lightweight, great middleware |
| **Database** | PostgreSQL | 16+ | MySQL, CockroachDB | pgvector support, materialized views, JSONB, robust |
| **Vector Search** | pgvector | 0.7+ | Pinecone, Weaviate | Same database, no extra infra, good enough for our scale |
| **Cache/Queue** | Redis | 7+ | RabbitMQ, NATS | Pub/sub + cache + rate limiting in one, simple |
| **ORM/Query** | sqlc | v2 | GORM, sqlx, Bun | Type-safe generated Go code from SQL, best performance |
| **Migrations** | goose | v3 | golang-migrate, atlas | Simple, Go-native, SQL migrations |
| **Frontend** | Next.js | 16+ (App Router) | Remix, SvelteKit | RSC, SSR, strong ecosystem, team familiarity |
| **UI Components** | shadcn/ui | latest | Radix, Ant Design | Owned code, Tailwind-native, excellent DX |
| **State (Server)** | TanStack Query | v5 | SWR | Optimistic updates, cache invalidation, SSE integration |
| **State (Client)** | Zustand | v5 | Jotai, Redux | Simple store pattern, minimal boilerplate |
| **Styling** | Tailwind CSS | v4 | CSS Modules, Emotion | Rapid prototyping, consistent design, great with shadcn |
| **Charts** | Recharts (shadcn) | v2 | Tremor, Chart.js | Tailwind integration via shadcn/ui Charts |
| **Forms** | React Hook Form + Zod | v7 / v3 | Formik | Minimal rerenders, schema-first validation |
| **Auth** | Clerk | v6 | Auth.js, Better Auth | Built-in RBAC, team management, org support |
| **Real-time** | SSE | native | WebSockets | Server→client only, simpler, auto-reconnect |
| **AI/LLM** | Claude API | Sonnet 4.6 / Haiku 4.5 | OpenAI, Gemini | Best reasoning quality, cost-effective |
| **Embeddings** | Voyage AI | voyage-3 | OpenAI ada-002, Cohere | Best retrieval quality, reasonable cost |
| **Extension** | Chrome MV3 + WXT | latest | Plasmo, CRXJS | File-based routing, HMR, TypeScript-first |
| **Extension UI** | React | 19 | Preact, Svelte | Same as dashboard, shared components |
| **Testing (Go)** | stdlib + testcontainers | - | Ginkgo | Table-driven tests, real DB in tests |
| **Testing (Frontend)** | Vitest + Playwright | v2 / v1 | Jest, Cypress | ESM-native, fast, real browser E2E |
| **CI/CD** | GitHub Actions | - | GitLab CI | GitHub-native, Railway integration |
| **Infrastructure** | Railway | - | Fly.io, Render | PostgreSQL + Redis + Go, deep expertise |
| **Monitoring** | OpenTelemetry + Grafana | - | Datadog, New Relic | Open-source, Railway compatible |
| **Email** | Resend | - | SendGrid, SES | Developer-friendly API, React Email |

---

## Backend Decisions (Detailed)

### Go + Chi Router

**Why Go:**
- Goroutines are natural for concurrent platform polling (each platform = independent goroutine)
- Built-in concurrency primitives (`sync.WaitGroup`, `errgroup`, channels)
- Single binary deployment — no runtime dependencies
- Low memory footprint (~10-20MB per service vs ~100MB+ for Node.js)
- Strong standard library (`net/http`, `encoding/json`, `context`)

**Why Chi over alternatives:**

| Framework | Pros | Cons | Decision |
|-----------|------|------|----------|
| **Chi v5** | stdlib compatible, lightweight, rich middleware, route groups | Less "batteries included" than Fiber | **Selected** |
| Fiber | Fastest benchmarks, Express-like API | Not `net/http` compatible, different handler signature | Rejected |
| Echo | Good middleware, built-in validation | Less active development than Chi | Rejected |
| Gin | Most popular, fast | Custom context, not stdlib compatible | Rejected |
| stdlib only | Zero dependencies | No route params, no middleware chain | Too bare |

Chi's stdlib compatibility means all standard `http.Handler` middleware works. This is critical for integrating third-party middleware (CORS, auth, compression).

### sqlc for Database Access

**Why sqlc over alternatives:**

| Library | Type Safety | Performance | Learning Curve | Decision |
|---------|------------|-------------|----------------|----------|
| **sqlc** | Full (generated) | Best (raw SQL) | Low (write SQL, get Go) | **Selected** |
| GORM | Partial (reflection) | Medium (ORM overhead) | Medium | Rejected |
| sqlx | Partial (struct tags) | Good (raw SQL) | Low | Runner-up |
| Bun | Good (Go generics) | Good | Medium | Considered |
| Ent | Full (code-gen) | Good | High (graph-based) | Too complex |

sqlc generates type-safe Go code from SQL queries. You write `.sql` files, and sqlc generates the Go structs and methods. This means:
- Zero runtime reflection
- Compile-time query validation
- IDE autocomplete on generated types
- Raw SQL performance with type safety

```sql
-- queries/mentions.sql
-- name: GetMentionsByKeyword :many
SELECT id, platform, content, relevance_score, created_at
FROM mentions
WHERE workspace_id = $1
  AND relevance_score >= $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
```

Generates:
```go
func (q *Queries) GetMentionsByKeyword(ctx context.Context, arg GetMentionsByKeywordParams) ([]Mention, error)
```

### goose for Migrations

Simple, Go-native migration tool. Supports both SQL and Go migrations. Integrates well with sqlc workflow:
1. Write migration SQL in `migrations/`
2. Run `goose up` to apply
3. Write queries in `queries/`
4. Run `sqlc generate` to get Go code

### Redis for Everything Async

Redis serves four roles:
1. **Pub/Sub**: Real-time event distribution between services
2. **Task Queue**: Using Redis Streams for reliable task processing
3. **Rate Limiting**: Token bucket per platform using `INCR` + `EXPIRE`
4. **Cache**: Mention deduplication, session data, API response caching

Single Redis instance handles all four. At scale (>1K users), separate into dedicated instances.

---

## Frontend Decisions (Detailed)

### Next.js 16 (App Router)

**Why Next.js:**
- Server Components reduce client bundle size (analytics, settings pages)
- Server Actions for form submissions and mutations
- Route Handlers for SSE endpoints
- Built-in image optimization, fonts, metadata
- Vercel deployment option (or self-host on Railway)

**App Router patterns for our dashboard:**
- `(auth)` route group: Login/signup pages with minimal layout
- `(dashboard)` route group: Full dashboard layout with sidebar + header
- `_components/` private folders: Colocate route-specific components
- `loading.tsx` per route: Skeleton UI while data loads

### shadcn/ui + Tailwind CSS v4

**Key components we'll use heavily:**

| Component | Use Case |
|-----------|----------|
| Command (cmdk) | Searchable mention inbox, quick actions |
| Data Table | Analytics tables, lead lists, mention lists |
| Sheet | Mention detail slide-over panel |
| Tabs | Pipeline stage views, analytics sections |
| Card | Dashboard metric cards, mention cards |
| Badge | Platform labels, status indicators |
| Dialog | Editing, file uploads, confirmations |
| Dropdown Menu | Context actions on mentions/leads |
| Calendar | Analytics date range picker |

**Tailwind CSS v4** advantages:
- CSS-first configuration (no `tailwind.config.js`)
- Lightning CSS engine for faster builds
- Container queries built-in
- OKLCH color space for better color palettes

**Supporting utilities:**
- `class-variance-authority (CVA)`: Component variant definitions
- `tailwind-merge`: Intelligent class merging
- `clsx`: Conditional class names
- Combined into the standard `cn()` utility function

### TanStack Query v5 + Zustand v5

**Data flow architecture:**

```
Server Data (mentions, leads, analytics)
    └── TanStack Query (fetch, cache, invalidate, optimistic update)

Real-time Updates (SSE events)
    └── TanStack Query cache mutations (setQueryData)

UI State (sidebar open, filters, modals)
    └── Zustand stores

URL State (filters, pagination, date range)
    └── nuqs (type-safe URL search params)

Form State (editing, creating)
    └── React Hook Form
```

### Clerk for Authentication

Clerk provides out-of-the-box:
- **Organizations**: Map to LeadEcho workspaces
- **Roles**: admin, editor (can reply), viewer (monitor-only)
- **Pre-built UI**: `<SignIn>`, `<UserButton>`, `<OrganizationSwitcher>`
- **Next.js middleware**: Route protection at the edge
- **Webhooks**: Sync user/org events with our database

Cost: Free for up to 10,000 MAU. $0.02/MAU after that. Well within budget.

### Recharts via shadcn/ui Charts

shadcn/ui wraps Recharts with consistent theming:
- Automatic color scheme from CSS variables
- Styled tooltips and legends
- Area, Bar, Line, Pie, Radar charts
- For advanced analytics, supplement with Tremor pre-built widgets

---

## AI/ML Decisions

### Claude API (Anthropic)

**Model selection by task:**

| Task | Model | Why | Est. Cost/1K calls |
|------|-------|-----|-------------------|
| Relevance scoring (1-10) | Haiku 4.5 | Fast, cheap, good enough for classification | ~$0.10 |
| Intent classification | Haiku 4.5 | Simple categorization | ~$0.10 |
| Thread analysis | Sonnet 4.6 | Needs nuanced understanding | ~$0.50 |
| Reply generation | Sonnet 4.6 | Quality matters for user-facing text | ~$0.80 |
| Persona matching | Sonnet 4.6 | Subtle voice matching | ~$0.80 |
| A/B variant generation | Sonnet 4.6 | Creative variation | ~$0.80 |

**Cost optimization:**
- Batch scoring: Score multiple mentions in one API call
- Cache embeddings: Don't re-embed unchanged documents
- Use Haiku for anything that doesn't need deep reasoning
- Set max_tokens appropriately (scoring needs ~50 tokens, replies need ~500)

### Voyage AI for Embeddings

**Why Voyage over alternatives:**

| Provider | Model | Dimensions | Quality (MTEB) | Cost/1M tokens |
|----------|-------|-----------|-----------------|----------------|
| **Voyage AI** | voyage-3 | 1024 | Top tier | $0.06 |
| OpenAI | text-embedding-3-small | 1536 | Good | $0.02 |
| OpenAI | text-embedding-3-large | 3072 | Better | $0.13 |
| Cohere | embed-v3 | 1024 | Good | $0.10 |

Voyage-3 offers the best retrieval quality at reasonable cost. 1024 dimensions is the sweet spot for pgvector (good quality without excessive storage/compute).

---

## Chrome Extension Decisions

### WXT Framework

**Why WXT over alternatives:**

| Framework | TypeScript | HMR | MV3 | Ecosystem | Decision |
|-----------|-----------|-----|-----|-----------|----------|
| **WXT** | First-class | Yes | Yes | Growing fast | **Selected** |
| Plasmo | Good | Yes | Yes | Larger | Runner-up |
| CRXJS | Good | Yes | Yes | Vite-based | Considered |
| Raw webpack | Manual | Manual | Manual | DIY | Too slow |

WXT advantages:
- File-based routing for content scripts and pages
- Auto-imports for browser APIs
- Built-in support for sidepanel, popup, content scripts
- React/Vue/Svelte support
- TypeScript-first with full type generation for `manifest.json`

### Manifest V3 Architecture

```
Extension Architecture:
├── Service Worker (background.ts)
│   ├── Chrome Extension API access
│   ├── Backend API communication
│   ├── Alarm-based periodic tasks
│   └── Message routing
├── Side Panel (sidepanel/)
│   ├── React UI (reply suggestions, mention feed)
│   ├── Reply editor with platform preview
│   └── Approval queue
├── Content Scripts
│   ├── linkedin.ts (LinkedIn feed monitoring)
│   ├── reddit.ts (Reply injection helper)
│   ├── twitter.ts (Reply injection helper)
│   └── human-mimicry.ts (Typing simulation)
└── Storage
    ├── chrome.storage.local (preferences, cache)
    └── chrome.storage.session (temporary auth state)
```

---

## Infrastructure Decisions

### Railway

**Why Railway:**
- Single platform for Go services + PostgreSQL + Redis
- Built-in PostgreSQL with pgvector extension support
- Private networking between services
- GitHub Actions deployment integration
- Environment variable management
- Custom domains + automatic SSL
- Reasonable pricing ($5/mo hobby, usage-based pro)

**Service architecture on Railway:**

```
Railway Project: leadecho
├── Service: api-gateway (Go binary)
├── Service: signal-engine (Go binary)  [Phase 3+]
├── Service: next-dashboard (Next.js)
├── Database: PostgreSQL (pgvector enabled)
├── Database: Redis
└── CronJob: analytics-refresh (materialized views)
```

Phase 1-2: Single Go binary (`api-gateway` includes signal engine + workflow engine)
Phase 3+: Split into separate services

### Vercel for Frontend (Alternative)

If Railway's Next.js performance isn't optimal, deploy the dashboard to Vercel:
- Edge Runtime for middleware (auth, redirects)
- ISR for marketing pages
- Serverless functions for API routes
- Deploy via `vercel.json` or GitHub integration

---

## Testing Strategy

### Go Backend

| Test Type | Tool | What to Test |
|-----------|------|-------------|
| Unit | `testing` stdlib | Business logic, scoring, parsing |
| Integration | testcontainers-go | Database queries, Redis pub/sub |
| HTTP | `httptest` | API endpoints, middleware |
| Mocks | `mockgen` (gomock) | External APIs (Claude, Reddit, etc.) |

**Best practices:**
- Table-driven tests for all scoring/parsing functions
- testcontainers-go for real PostgreSQL + Redis in CI
- `httptest.NewServer` for API endpoint tests
- Mock external APIs (never call Claude/Reddit in tests)

### Frontend

| Test Type | Tool | What to Test |
|-----------|------|-------------|
| Unit | Vitest | Utilities, Zod schemas, data transforms |
| Component | Vitest + Testing Library | UI components, form validation |
| Integration | Vitest + MSW | API integration, SSE handling |
| E2E | Playwright | Critical flows (login, inbox, reply) |
| Accessibility | axe-playwright | WCAG compliance |

---

## Dependency Inventory

### Go Dependencies (`go.mod`)

```
github.com/go-chi/chi/v5           # HTTP router
github.com/go-chi/cors              # CORS middleware
github.com/redis/go-redis/v9        # Redis client
github.com/jackc/pgx/v5             # PostgreSQL driver
github.com/pgvector/pgvector-go     # pgvector support
github.com/pressly/goose/v3         # Migrations
github.com/anthropics/anthropic-go  # Claude API
github.com/golang-jwt/jwt/v5        # JWT handling
github.com/rs/zerolog               # Structured logging
github.com/sethvargo/go-envconfig   # Config from env vars
golang.org/x/sync                   # errgroup
golang.org/x/time                   # rate limiter
```

### Frontend Dependencies (`package.json`)

```json
{
  "dependencies": {
    "next": "^16",
    "react": "^19",
    "@tanstack/react-query": "^5",
    "zustand": "^5",
    "nuqs": "^2",
    "react-hook-form": "^7",
    "@hookform/resolvers": "^3",
    "zod": "^3",
    "recharts": "^2",
    "@dnd-kit/core": "^6",
    "@dnd-kit/sortable": "^8",
    "sonner": "^1",
    "@clerk/nextjs": "^6",
    "date-fns": "^3",
    "clsx": "^2",
    "tailwind-merge": "^2",
    "class-variance-authority": "^0.7"
  },
  "devDependencies": {
    "typescript": "^5.7",
    "tailwindcss": "^4",
    "vitest": "^2",
    "@testing-library/react": "^16",
    "@playwright/test": "^1",
    "msw": "^2"
  }
}
```

### Chrome Extension Dependencies

```json
{
  "dependencies": {
    "wxt": "^0.19",
    "react": "^19",
    "react-dom": "^19",
    "zustand": "^5",
    "zod": "^3"
  }
}
```
