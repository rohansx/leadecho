# LeadEcho - Infrastructure & Deployment Guide

## Overview

LeadEcho runs on Railway for backend services and optionally Vercel for the Next.js dashboard. This guide covers Docker configuration, CI/CD pipelines, database management, monitoring, scaling strategy, and disaster recovery.

---

## Infrastructure Layout

```
┌─────────────────────────────────────────────────────┐
│ Railway Project: leadecho                          │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐                │
│  │ Go API       │  │ Next.js      │                │
│  │ (+ Signal    │  │ Dashboard    │                │
│  │  Engine)     │  │              │                │
│  │ Port 8080    │  │ Port 3000    │                │
│  └──────┬───────┘  └──────┬───────┘                │
│         │                  │                        │
│  ┌──────┴──────────────────┴───────┐                │
│  │ Private Network (internal DNS)  │                │
│  └──────┬──────────────────┬───────┘                │
│         │                  │                        │
│  ┌──────┴───────┐  ┌──────┴───────┐                │
│  │ PostgreSQL   │  │ Redis        │                │
│  │ + pgvector   │  │              │                │
│  │ Port 5432    │  │ Port 6379    │                │
│  └──────────────┘  └──────────────┘                │
└─────────────────────────────────────────────────────┘
```

### Service Topology

| Service | Type | Resources | Notes |
|---------|------|-----------|-------|
| Go API | Custom Docker | 1 vCPU, 1GB RAM | Includes Signal Engine |
| Next.js Dashboard | Custom Docker | 0.5 vCPU, 512MB RAM | Or use Vercel |
| PostgreSQL | Railway Plugin | 1GB RAM, 10GB storage | pgvector extension |
| Redis | Railway Plugin | 256MB RAM | Persistence: RDB + AOF |

---

## Railway Service Configuration

### railway.toml (Go API)

```toml
[build]
builder = "dockerfile"
dockerfilePath = "backend/Dockerfile"

[deploy]
healthcheckPath = "/healthz"
healthcheckTimeout = 10
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 5
numReplicas = 1

[service]
internalPort = 8080
```

### railway.toml (Next.js)

```toml
[build]
builder = "dockerfile"
dockerfilePath = "dashboard/Dockerfile"

[deploy]
healthcheckPath = "/api/health"
healthcheckTimeout = 10
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 3
numReplicas = 1

[service]
internalPort = 3000
```

---

## Docker Configuration

### Go API Dockerfile

```dockerfile
# backend/Dockerfile
# Stage 1: Build
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o /app/leadecho ./cmd/api

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/leadecho /leadecho
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/leadecho"]
```

**Final image size:** ~12-16MB (distroless, statically linked)

### Next.js Dashboard Dockerfile

```dockerfile
# dashboard/Dockerfile
FROM node:22-alpine AS base

# Stage 1: Dependencies
FROM base AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN corepack enable pnpm && pnpm install --frozen-lockfile

# Stage 2: Build
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
RUN corepack enable pnpm && pnpm build

# Stage 3: Runtime
FROM base AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
```

---

## Environment Variables

### Complete Variable Reference

| Variable | Service | Required | Description |
|----------|---------|----------|-------------|
| `DATABASE_URL` | Go API | Yes | PostgreSQL connection string |
| `REDIS_URL` | Go API | Yes | Redis connection string |
| `PORT` | Go API | Yes | HTTP port (8080) |
| `CLERK_SECRET_KEY` | Go API | Yes | Clerk backend API key |
| `CLERK_WEBHOOK_SECRET` | Go API | Yes | Clerk webhook signing secret |
| `ANTHROPIC_API_KEY` | Go API | Yes | Claude API key |
| `VOYAGE_API_KEY` | Go API | Yes | Voyage AI embedding key |
| `STRIPE_SECRET_KEY` | Go API | Yes | Stripe API key |
| `STRIPE_WEBHOOK_SECRET` | Go API | Yes | Stripe webhook signing secret |
| `REDDIT_CLIENT_ID` | Go API | Cond. | Reddit OAuth2 app ID |
| `REDDIT_CLIENT_SECRET` | Go API | Cond. | Reddit OAuth2 secret |
| `TWITTER_BEARER_TOKEN` | Go API | Cond. | X/Twitter API v2 bearer |
| `SLACK_BOT_TOKEN` | Go API | No | Slack bot OAuth token |
| `SLACK_SIGNING_SECRET` | Go API | No | Slack request signing secret |
| `UTM_REDIRECT_BASE_URL` | Go API | Yes | e.g., `https://r.leadecho.app` |
| `LOG_LEVEL` | Go API | No | debug/info/warn/error (default: info) |
| `ENVIRONMENT` | Go API | No | development/staging/production |
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | Dashboard | Yes | Clerk frontend key |
| `NEXT_PUBLIC_API_URL` | Dashboard | Yes | Go API URL |

### .env.example

```bash
# Database
DATABASE_URL=postgres://user:pass@localhost:5432/leadecho?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379/0

# Server
PORT=8080
ENVIRONMENT=development
LOG_LEVEL=debug

# Auth (Clerk)
CLERK_SECRET_KEY=sk_test_...
CLERK_WEBHOOK_SECRET=whsec_...

# AI
ANTHROPIC_API_KEY=sk-ant-...
VOYAGE_API_KEY=pa-...

# Payments
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...

# Platform APIs
REDDIT_CLIENT_ID=
REDDIT_CLIENT_SECRET=
TWITTER_BEARER_TOKEN=

# Notifications
SLACK_BOT_TOKEN=xoxb-...
SLACK_SIGNING_SECRET=

# Links
UTM_REDIRECT_BASE_URL=http://localhost:8080
```

---

## CI/CD Pipeline (GitHub Actions)

### Main Workflow

```yaml
# .github/workflows/ci.yml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  release:
    types: [published]

env:
  GO_VERSION: '1.23'
  NODE_VERSION: '22'

jobs:
  # Parallel lint jobs
  lint-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          working-directory: backend
          version: latest

  lint-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
      - name: Install pnpm
        run: corepack enable pnpm
      - name: Install dependencies
        run: cd dashboard && pnpm install --frozen-lockfile
      - name: Lint
        run: cd dashboard && pnpm lint
      - name: Type check
        run: cd dashboard && pnpm type-check

  # Go tests with services
  test-go:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: leadecho_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
      - name: Run migrations
        run: |
          cd backend
          go install github.com/pressly/goose/v3/cmd/goose@latest
          goose -dir migrations postgres "postgres://test:test@localhost:5432/leadecho_test?sslmode=disable" up
      - name: Run tests
        env:
          DATABASE_URL: postgres://test:test@localhost:5432/leadecho_test?sslmode=disable
          REDIS_URL: redis://localhost:6379/0
        run: cd backend && go test -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: backend/coverage.out

  # Frontend tests
  test-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
      - name: Install pnpm
        run: corepack enable pnpm
      - name: Install dependencies
        run: cd dashboard && pnpm install --frozen-lockfile
      - name: Run tests
        run: cd dashboard && pnpm test

  # Deploy to staging on push to main
  deploy-staging:
    needs: [lint-go, lint-frontend, test-go, test-frontend]
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - uses: actions/checkout@v4
      - name: Install Railway CLI
        run: npm install -g @railway/cli
      - name: Deploy Go API to staging
        run: railway up --service leadecho-api --environment staging
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN }}
      - name: Deploy Dashboard to staging
        run: railway up --service leadecho-dashboard --environment staging
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN }}
      - name: Health check
        run: |
          sleep 30
          curl -f https://staging-api.leadecho.app/healthz || exit 1

  # Deploy to production on release
  deploy-production:
    needs: [lint-go, lint-frontend, test-go, test-frontend]
    if: github.event_name == 'release'
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      - name: Install Railway CLI
        run: npm install -g @railway/cli
      - name: Deploy Go API
        run: railway up --service leadecho-api --environment production
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN_PROD }}
      - name: Deploy Dashboard
        run: railway up --service leadecho-dashboard --environment production
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN_PROD }}
      - name: Health check
        run: |
          sleep 30
          STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://api.leadecho.app/healthz)
          if [ "$STATUS" != "200" ]; then
            echo "Health check failed with status $STATUS"
            railway rollback --service leadecho-api --environment production
            exit 1
          fi
      - name: Notify Slack
        if: always()
        uses: slackapi/slack-github-action@v1
        with:
          payload: |
            {
              "text": "Deploy ${{ job.status }}: LeadEcho ${{ github.event.release.tag_name }}"
            }
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_DEPLOY_WEBHOOK }}
```

---

## Database Management

### Migration Workflow

```bash
# Create new migration
cd backend
goose -dir migrations create add_workflows_table sql

# Run migrations (local)
goose -dir migrations postgres "$DATABASE_URL" up

# Check status
goose -dir migrations postgres "$DATABASE_URL" status

# Rollback last
goose -dir migrations postgres "$DATABASE_URL" down
```

### pgvector Setup

```sql
-- First migration: enable extensions
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- After loading data, create indexes
CREATE INDEX idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

### Backup Strategy

```yaml
# Railway provides automatic daily backups for PostgreSQL.
# Additional manual backup schedule:
# - Daily: Railway automatic snapshots
# - Weekly: pg_dump to S3 (if configured)
# - Before migrations: Manual snapshot via Railway dashboard
```

### Connection Pooling

```go
// Use pgx pool with sensible defaults
func NewDBPool(databaseURL string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, err
    }

    config.MaxConns = 20
    config.MinConns = 5
    config.MaxConnLifetime = 30 * time.Minute
    config.MaxConnIdleTime = 5 * time.Minute
    config.HealthCheckPeriod = 30 * time.Second

    return pgxpool.NewWithConfig(context.Background(), config)
}
```

### Materialized View Refresh

```go
// Refresh materialized views every 15 minutes
func (s *Scheduler) StartViewRefresh(ctx context.Context) error {
    ticker := time.NewTicker(15 * time.Minute)
    defer ticker.Stop()

    views := []string{
        "mv_mention_stats_daily",
        "mv_lead_funnel",
        "mv_reply_performance",
        "mv_keyword_performance",
    }

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            for _, view := range views {
                _, err := s.db.Exec(ctx, fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", view))
                if err != nil {
                    s.logger.Error().Err(err).Str("view", view).Msg("failed to refresh materialized view")
                }
            }
        }
    }
}
```

---

## Monitoring & Observability

### Health Check Endpoint

```go
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    status := map[string]string{
        "status":   "ok",
        "version":  version,
        "postgres": "ok",
        "redis":    "ok",
    }

    // Check PostgreSQL
    if err := h.db.Ping(ctx); err != nil {
        status["postgres"] = "error"
        status["status"] = "degraded"
    }

    // Check Redis
    if err := h.redis.Ping(ctx).Err(); err != nil {
        status["redis"] = "error"
        status["status"] = "degraded"
    }

    code := http.StatusOK
    if status["status"] != "ok" {
        code = http.StatusServiceUnavailable
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(status)
}
```

### Structured Logging (zerolog)

```go
func SetupLogger(env string) zerolog.Logger {
    level := zerolog.InfoLevel
    if env == "development" {
        level = zerolog.DebugLevel
    }

    return zerolog.New(os.Stdout).
        Level(level).
        With().
        Timestamp().
        Str("service", "leadecho-api").
        Logger()
}

// Request logging middleware
func RequestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            defer func() {
                logger.Info().
                    Str("method", r.Method).
                    Str("path", r.URL.Path).
                    Int("status", ww.Status()).
                    Dur("duration", time.Since(start)).
                    Int("bytes", ww.BytesWritten()).
                    Str("ip", r.RemoteAddr).
                    Msg("request")
            }()

            next.ServeHTTP(ww, r)
        })
    }
}
```

### Key Metrics & Alerts

| Metric | Target | Warning | Critical |
|--------|--------|---------|----------|
| API response time (p95) | < 500ms | > 1s | > 3s |
| API error rate | < 1% | > 2% | > 5% |
| Signal Engine poll success | > 95% | < 90% | < 80% |
| DB connection pool usage | < 60% | > 75% | > 90% |
| Redis memory usage | < 70% | > 80% | > 90% |
| AI API latency (p95) | < 5s | > 8s | > 15s |
| Mention processing lag | < 5min | > 15min | > 30min |
| Reply removal rate | < 5% | > 10% | > 20% |

---

## Scaling Strategy

### Phase 1: Single Binary (0-500 users)

```
Railway Project
├── Go API (single binary: API + Signal Engine + Workflow Engine)
│   └── 1 vCPU, 1GB RAM
├── Next.js Dashboard
│   └── 0.5 vCPU, 512MB RAM
├── PostgreSQL (Railway plugin)
│   └── 1GB RAM, 10GB storage
└── Redis (Railway plugin)
    └── 256MB RAM
```

**Monthly cost:** $200-350

### Phase 2: Separate Workers (500-2,000 users)

```
Railway Project
├── Go API (HTTP only)
│   └── 1 vCPU, 1GB RAM
├── Signal Engine Worker (polling + scoring)
│   └── 1 vCPU, 1GB RAM
├── Next.js Dashboard
│   └── 0.5 vCPU, 512MB RAM
├── PostgreSQL + Read Replica
│   └── 2GB RAM, 25GB storage
└── Redis
    └── 512MB RAM
```

**Monthly cost:** $500-800

### Phase 3: Horizontal Scale (2,000+ users)

```
Railway Project
├── Go API (2 replicas, load balanced)
├── Signal Engine (1-2 replicas)
├── Workflow Workers (2-4 replicas)
├── Next.js Dashboard (2 replicas)
├── PostgreSQL Primary + 2 Read Replicas
│   └── 4GB RAM, 50GB+ storage
├── Redis Cluster (or Upstash)
│   └── 1GB RAM
└── S3 (document uploads)
```

**Monthly cost:** $1,000-2,000

### Cost Optimization

1. **Use Haiku for scoring** - 10x cheaper than Sonnet, good enough for classification
2. **Cache embeddings** - Don't re-embed unchanged documents
3. **Batch AI calls** - Score 10 mentions per API call
4. **Only generate replies for high-score mentions** - Score >= 7.0 (top ~10%)
5. **Materialized views** - Pre-compute analytics instead of real-time aggregation
6. **Connection pooling** - pgx pool with MinConns to avoid cold start overhead

---

## Custom Domains

### Subdomain Strategy

| Domain | Service | Purpose |
|--------|---------|---------|
| `app.leadecho.app` | Next.js Dashboard | Main web app |
| `api.leadecho.app` | Go API | REST API |
| `r.leadecho.app` | Go API (UTM handler) | Link redirects |

### Railway Custom Domain Setup

```bash
# Add custom domain in Railway dashboard or CLI
railway domain add api.leadecho.app --service leadecho-api
railway domain add app.leadecho.app --service leadecho-dashboard
railway domain add r.leadecho.app --service leadecho-api
```

Railway provides automatic SSL via Let's Encrypt.

---

## Environment Parity

### Dev vs Staging vs Production

| Aspect | Development | Staging | Production |
|--------|-------------|---------|------------|
| Database | Local Docker PG | Railway PG (staging) | Railway PG (prod) |
| Redis | Local Docker Redis | Railway Redis (staging) | Railway Redis (prod) |
| AI APIs | Real APIs (low usage) | Real APIs | Real APIs |
| Auth | Clerk dev instance | Clerk staging | Clerk production |
| Payments | Stripe test mode | Stripe test mode | Stripe live mode |
| Logging | Debug level, console | Info level, JSON | Info level, JSON |
| URL | localhost:8080 | staging-api.leadecho.app | api.leadecho.app |

### Docker Compose (Local Development)

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

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --save 60 1 --loglevel warning

volumes:
  pgdata:
```

---

## Disaster Recovery

### Recovery Objectives

| Metric | Target |
|--------|--------|
| RTO (Recovery Time Objective) | < 1 hour |
| RPO (Recovery Point Objective) | < 15 minutes |

### Recovery Scenarios

| Scenario | Recovery Steps |
|----------|---------------|
| Service crash | Railway auto-restarts (restartPolicyType: on_failure) |
| Bad deployment | Railway rollback to previous deployment |
| Database corruption | Restore from Railway automatic backup |
| Redis data loss | Redis rebuilds from PostgreSQL (source of truth) |
| Full Railway outage | Wait for Railway recovery; data persisted in managed PG |

### Pre-Deployment Checklist

- [ ] All tests pass in CI
- [ ] Database migrations tested in staging
- [ ] Environment variables confirmed for target environment
- [ ] Health check endpoint verified on staging
- [ ] Rollback plan documented
- [ ] Team notified of deployment window
