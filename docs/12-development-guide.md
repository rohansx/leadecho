# LeadEcho - Development Setup & Conventions Guide

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23+ | Backend API, Signal Engine, Workflow Engine |
| Node.js | 22 LTS | Dashboard (Next.js), Extension (WXT) |
| pnpm | 9+ | Node package manager |
| Docker | 24+ | Local PostgreSQL + Redis |
| Docker Compose | 2.x | Multi-container orchestration |
| goose | v3 | Database migrations |
| sqlc | 1.27+ | Type-safe SQL code generation |
| golangci-lint | latest | Go linting |

### Installation

```bash
# Go tools
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Node tools
corepack enable pnpm

# Verify
go version          # go1.23.x
node --version      # v22.x.x
pnpm --version      # 9.x.x
docker --version    # Docker 24.x+
goose --version     # goose v3.x.x
sqlc version        # v1.27.x
```

---

## Repository Structure

```
leadecho/
├── backend/                    # Go monolith
│   ├── cmd/
│   │   └── api/
│   │       └── main.go         # Entry point
│   ├── internal/
│   │   ├── api/                # HTTP handlers + middleware
│   │   │   ├── handler/
│   │   │   ├── middleware/
│   │   │   └── router.go
│   │   ├── database/           # sqlc generated code + queries
│   │   │   ├── queries/        # .sql query files
│   │   │   ├── models.go       # Generated models
│   │   │   └── querier.go      # Generated interface
│   │   ├── platform/           # Platform adapters
│   │   │   ├── adapter.go      # Interface
│   │   │   ├── hackernews.go
│   │   │   ├── reddit.go
│   │   │   ├── twitter.go
│   │   │   └── linkedin.go
│   │   ├── signal/             # Signal engine orchestrator
│   │   ├── workflow/           # Workflow engine
│   │   ├── rag/                # RAG pipeline
│   │   │   ├── chunker.go
│   │   │   ├── embedder.go
│   │   │   ├── retriever.go
│   │   │   └── generator.go
│   │   ├── safe/               # Safe engagement system
│   │   └── config/             # Configuration loading
│   ├── migrations/             # goose SQL migrations
│   ├── sqlc.yaml               # sqlc configuration
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── dashboard/                  # Next.js 16 app
│   ├── src/
│   │   ├── app/                # App Router pages
│   │   ├── components/         # React components
│   │   ├── lib/                # Utilities, API client
│   │   ├── hooks/              # Custom React hooks
│   │   └── stores/             # Zustand stores
│   ├── public/
│   ├── package.json
│   ├── next.config.ts
│   ├── tailwind.config.ts
│   ├── tsconfig.json
│   └── Dockerfile
├── extension/                  # Chrome Extension (WXT)
│   ├── entrypoints/
│   │   ├── background.ts
│   │   ├── sidepanel/
│   │   └── content/
│   ├── components/
│   ├── lib/
│   ├── wxt.config.ts
│   ├── package.json
│   └── tsconfig.json
├── docs/                       # Architecture & design docs
├── docker-compose.yml          # Local dev services
├── Makefile                    # Development commands
├── .github/
│   └── workflows/
│       └── ci.yml              # CI/CD pipeline
├── .env.example
└── README.md
```

---

## Local Development Setup

### 1. Clone and Install

```bash
git clone https://github.com/your-org/leadecho.git
cd leadecho

# Copy environment file
cp .env.example .env
# Edit .env with your API keys (Clerk, Anthropic, Voyage, etc.)
```

### 2. Start Infrastructure

```bash
# Start PostgreSQL + Redis
docker compose up -d

# Wait for services to be healthy
docker compose ps
```

### 3. Run Migrations

```bash
# Apply all migrations
cd backend
goose -dir migrations postgres "postgres://leadecho:leadecho@localhost:5432/leadecho_dev?sslmode=disable" up

# Generate sqlc code
sqlc generate
```

### 4. Start Services

```bash
# Terminal 1: Go API
cd backend
go run ./cmd/api

# Terminal 2: Next.js Dashboard
cd dashboard
pnpm install
pnpm dev

# Terminal 3: Chrome Extension
cd extension
pnpm install
pnpm dev
```

### Quick Start with Makefile

```bash
make setup    # Install deps + start Docker + run migrations
make dev      # Start all services (uses tmux or separate terminals)
make test     # Run all tests
make lint     # Lint all code
```

---

## Docker Compose (Local)

```yaml
# docker-compose.yml
services:
  postgres:
    image: pgvector/pgvector:pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: leadecho
      POSTGRES_PASSWORD: leadecho
      POSTGRES_DB: leadecho_dev
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U leadecho"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --save 60 1 --loglevel warning
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pgdata:
```

---

## Makefile

```makefile
.PHONY: setup dev test lint migrate sqlc build clean

# Development setup
setup:
	docker compose up -d
	@echo "Waiting for PostgreSQL..."
	@sleep 3
	cd backend && goose -dir migrations postgres "$(DATABASE_URL)" up
	cd backend && sqlc generate
	cd dashboard && pnpm install
	cd extension && pnpm install

# Run all services
dev-api:
	cd backend && go run ./cmd/api

dev-dashboard:
	cd dashboard && pnpm dev

dev-extension:
	cd extension && pnpm dev

# Testing
test: test-go test-dashboard

test-go:
	cd backend && go test -race -count=1 ./...

test-dashboard:
	cd dashboard && pnpm test

test-cover:
	cd backend && go test -race -coverprofile=coverage.out ./...
	cd backend && go tool cover -html=coverage.out -o coverage.html

# Linting
lint: lint-go lint-dashboard

lint-go:
	cd backend && golangci-lint run ./...

lint-dashboard:
	cd dashboard && pnpm lint && pnpm type-check

# Database
migrate-up:
	cd backend && goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	cd backend && goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-create:
	cd backend && goose -dir migrations create $(name) sql

migrate-status:
	cd backend && goose -dir migrations postgres "$(DATABASE_URL)" status

# Code generation
sqlc:
	cd backend && sqlc generate

# Build
build-api:
	cd backend && CGO_ENABLED=0 go build -o bin/leadecho ./cmd/api

build-dashboard:
	cd dashboard && pnpm build

build-extension:
	cd extension && pnpm build

# Clean
clean:
	docker compose down -v
	rm -rf backend/bin
	rm -rf dashboard/.next
	rm -rf extension/.output
```

---

## sqlc Configuration

```yaml
# backend/sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/database/queries/"
    schema: "migrations/"
    gen:
      go:
        package: "database"
        out: "internal/database"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: true
        emit_exact_table_names: false
        emit_empty_slices: true
        overrides:
          - db_type: "uuid"
            go_type: "string"
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "vector"
            go_type:
              import: "github.com/pgvector/pgvector-go"
              type: "Vector"
```

### Query File Conventions

```sql
-- internal/database/queries/mentions.sql

-- name: ListMentions :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
  AND (@status::mention_status IS NULL OR status = @status)
  AND (@platform::platform_type IS NULL OR platform = @platform)
  AND (@min_score::float IS NULL OR relevance_score >= @min_score)
ORDER BY
  CASE WHEN @sort = 'relevance_score' THEN relevance_score END DESC,
  CASE WHEN @sort != 'relevance_score' THEN created_at END DESC
LIMIT @page_limit;

-- name: GetMentionByID :one
SELECT * FROM mentions
WHERE id = @id AND workspace_id = @workspace_id;

-- name: UpdateMentionStatus :exec
UPDATE mentions
SET status = @status, updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id;
```

---

## Go Conventions

### Project Layout

```
internal/           # Private application code
├── api/            # HTTP layer
│   ├── handler/    # Request handlers (thin - delegate to services)
│   ├── middleware/  # Auth, logging, rate limiting
│   └── router.go   # Route definitions
├── database/       # Data layer (sqlc generated)
├── platform/       # External platform integrations
├── signal/         # Signal engine business logic
├── workflow/       # Workflow engine business logic
├── rag/            # RAG pipeline business logic
├── safe/           # Safety rules and engagement limits
└── config/         # Environment config loading
```

### Error Handling

```go
// Wrap errors with context using fmt.Errorf
func (h *Handler) CreateKeyword(ctx context.Context, req CreateKeywordRequest) (*Keyword, error) {
    if err := validateCreateKeyword(req); err != nil {
        return nil, fmt.Errorf("validate: %w", err)
    }

    keyword, err := h.db.InsertKeyword(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("insert keyword: %w", err)
    }

    return keyword, nil
}

// Use errors.Is / errors.As for type checking
if errors.Is(err, database.ErrNotFound) {
    respondError(w, http.StatusNotFound, "NOT_FOUND", "keyword not found")
    return
}
```

### Logging

```go
// Use zerolog with structured fields
h.logger.Info().
    Str("workspace_id", workspaceID).
    Str("platform", platform).
    Int("mention_count", len(mentions)).
    Dur("duration", time.Since(start)).
    Msg("poll completed")

// Error logging includes stack context
h.logger.Error().
    Err(err).
    Str("handler", "CreateKeyword").
    Str("workspace_id", workspaceID).
    Msg("failed to create keyword")
```

### Configuration

```go
// internal/config/config.go
package config

import "github.com/sethvargo/go-envconfig"

type Config struct {
    Port        int    `env:"PORT,default=8080"`
    Environment string `env:"ENVIRONMENT,default=development"`
    LogLevel    string `env:"LOG_LEVEL,default=info"`

    DatabaseURL string `env:"DATABASE_URL,required"`
    RedisURL    string `env:"REDIS_URL,required"`

    ClerkSecretKey     string `env:"CLERK_SECRET_KEY,required"`
    ClerkWebhookSecret string `env:"CLERK_WEBHOOK_SECRET,required"`

    AnthropicAPIKey string `env:"ANTHROPIC_API_KEY,required"`
    VoyageAPIKey    string `env:"VOYAGE_API_KEY,required"`

    StripeSecretKey     string `env:"STRIPE_SECRET_KEY,required"`
    StripeWebhookSecret string `env:"STRIPE_WEBHOOK_SECRET,required"`

    UTMRedirectBaseURL string `env:"UTM_REDIRECT_BASE_URL,required"`
}

func Load(ctx context.Context) (*Config, error) {
    var cfg Config
    if err := envconfig.Process(ctx, &cfg); err != nil {
        return nil, fmt.Errorf("load config: %w", err)
    }
    return &cfg, nil
}
```

### Testing

```go
// Use testify for assertions
func TestScoreBatch(t *testing.T) {
    scorer := NewScorer(mockClaude)

    mentions := []RawMention{
        {Content: "Looking for a CRM alternative", Platform: "reddit"},
    }

    results, err := scorer.ScoreBatch(context.Background(), mentions)
    require.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Greater(t, results[0].RelevanceScore, 5.0)
}

// Use testcontainers for integration tests
func TestMentionRepository(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    ctx := context.Background()
    pg := setupTestPostgres(t) // testcontainers
    db := database.New(pg)

    // ... test database operations
}
```

---

## Frontend Conventions

### Component Structure

```
src/
├── app/                    # Next.js App Router
│   ├── (dashboard)/        # Authenticated layout group
│   │   ├── mentions/
│   │   │   └── page.tsx
│   │   ├── leads/
│   │   │   └── page.tsx
│   │   └── layout.tsx
│   ├── sign-in/
│   ├── sign-up/
│   └── layout.tsx
├── components/
│   ├── ui/                 # shadcn/ui primitives
│   ├── mentions/           # Feature-specific components
│   │   ├── mention-card.tsx
│   │   ├── mention-filters.tsx
│   │   └── mention-detail.tsx
│   ├── leads/
│   └── shared/             # Cross-feature components
├── hooks/
│   ├── use-mentions.ts     # TanStack Query hooks
│   ├── use-leads.ts
│   └── use-sse.ts          # SSE connection hook
├── lib/
│   ├── api.ts              # API client (fetch wrapper)
│   ├── utils.ts
│   └── constants.ts
└── stores/
    └── ui-store.ts         # Zustand for UI state
```

### API Client

```typescript
// src/lib/api.ts
const API_BASE = process.env.NEXT_PUBLIC_API_URL;

class APIClient {
    private async fetch<T>(path: string, options?: RequestInit): Promise<T> {
        const token = await getToken(); // Clerk

        const res = await fetch(`${API_BASE}${path}`, {
            ...options,
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`,
                ...options?.headers,
            },
        });

        if (!res.ok) {
            const error = await res.json();
            throw new APIError(res.status, error);
        }

        return res.json();
    }

    mentions = {
        list: (params?: MentionFilters) => this.fetch<PaginatedResponse<Mention>>(`/api/v1/mentions?${toQueryString(params)}`),
        get: (id: string) => this.fetch<Mention>(`/api/v1/mentions/${id}`),
        update: (id: string, data: Partial<Mention>) => this.fetch<Mention>(`/api/v1/mentions/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
    };

    leads = {
        list: (params?: LeadFilters) => this.fetch<PaginatedResponse<Lead>>(`/api/v1/leads?${toQueryString(params)}`),
        create: (data: CreateLead) => this.fetch<Lead>('/api/v1/leads', { method: 'POST', body: JSON.stringify(data) }),
    };
}

export const api = new APIClient();
```

### TanStack Query Hooks

```typescript
// src/hooks/use-mentions.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';

export function useMentions(filters?: MentionFilters) {
    return useQuery({
        queryKey: ['mentions', filters],
        queryFn: () => api.mentions.list(filters),
    });
}

export function useUpdateMention() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: ({ id, data }: { id: string; data: Partial<Mention> }) =>
            api.mentions.update(id, data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['mentions'] });
        },
    });
}
```

### Naming Conventions

| Type | Convention | Example |
|------|-----------|---------|
| Components | PascalCase | `MentionCard.tsx` |
| Files | kebab-case | `mention-card.tsx` |
| Hooks | camelCase with `use` prefix | `useMentions` |
| Stores | camelCase with `Store` suffix | `uiStore` |
| API types | PascalCase | `Mention`, `Lead` |
| Constants | SCREAMING_SNAKE | `MAX_PAGE_SIZE` |

---

## Git Workflow

### Branch Naming

```
feature/add-workflow-engine
fix/reddit-rate-limit-handling
chore/update-dependencies
docs/api-specification
```

### Commit Messages

```
feat: add workflow engine with Redis Streams task queue
fix: handle Reddit 429 rate limit response correctly
chore: update Go dependencies to latest versions
docs: add API design specification
test: add integration tests for mention scoring
refactor: extract platform adapter interface
```

Follow [Conventional Commits](https://www.conventionalcommits.org/) format:
- `feat:` new feature
- `fix:` bug fix
- `chore:` maintenance
- `docs:` documentation
- `test:` tests
- `refactor:` refactoring (no behavior change)

### PR Process

1. Create feature branch from `main`
2. Push and open PR with description
3. CI must pass (lint + test)
4. Code review required
5. Squash merge to `main`
6. Auto-deploy to staging

---

## Debugging Tips

### Go API

```bash
# Run with debug logging
LOG_LEVEL=debug go run ./cmd/api

# Run specific test with verbose output
go test -v -run TestScoreBatch ./internal/signal/...

# Profile memory/CPU
go test -bench=. -benchmem ./internal/rag/...
```

### Database

```bash
# Connect to local PostgreSQL
psql postgres://leadecho:leadecho@localhost:5432/leadecho_dev

# Check migration status
goose -dir migrations postgres "$DATABASE_URL" status

# Reset database
goose -dir migrations postgres "$DATABASE_URL" reset
goose -dir migrations postgres "$DATABASE_URL" up
sqlc generate
```

### Redis

```bash
# Connect to local Redis
redis-cli

# Monitor all commands in real-time
redis-cli MONITOR

# Check stream contents
redis-cli XLEN stream:workflow:executions
redis-cli XRANGE stream:workflow:executions - + COUNT 10

# Check pub/sub
redis-cli SUBSCRIBE mention.detected mention.scored
```

### Chrome Extension

```bash
# Build and watch for changes
cd extension && pnpm dev

# Load in Chrome:
# 1. Go to chrome://extensions
# 2. Enable Developer mode
# 3. Click "Load unpacked"
# 4. Select extension/.output/chrome-mv3

# Debug service worker:
# Click "Inspect views: service worker" in chrome://extensions

# Debug content script:
# Open DevTools on target page → Sources → Content scripts
```

### Next.js Dashboard

```bash
# Run with debug info
cd dashboard && pnpm dev

# Check bundle size
pnpm build && npx @next/bundle-analyzer
```
