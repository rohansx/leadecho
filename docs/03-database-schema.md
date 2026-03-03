# LeadEcho - Database Schema Design

## Schema Overview

PostgreSQL 16+ with pgvector extension. All queries generated via sqlc. Migrations managed by goose.

```
workspaces ──┬── keywords
             ├── users
             ├── mentions ──┬── threads
             │              ├── replies
             │              └── leads ── lead_events
             ├── documents ── document_chunks (pgvector)
             ├── workflows ── workflow_executions
             ├── utm_links ── utm_events
             ├── platform_accounts
             ├── notifications
             └── subscriptions ── subscription_plans
```

---

## Enums

```sql
CREATE TYPE platform_type AS ENUM (
    'hackernews', 'reddit', 'twitter', 'linkedin'
);

CREATE TYPE mention_status AS ENUM (
    'new', 'reviewed', 'replied', 'archived', 'spam'
);

CREATE TYPE intent_type AS ENUM (
    'buy_signal', 'complaint', 'recommendation_ask', 'comparison', 'general'
);

CREATE TYPE lead_stage AS ENUM (
    'prospect', 'qualified', 'engaged', 'converted', 'lost'
);

CREATE TYPE reply_status AS ENUM (
    'draft', 'approved', 'posted', 'failed'
);

CREATE TYPE reply_variant AS ENUM (
    'value_only', 'technical', 'soft_sell'
);

CREATE TYPE workflow_status AS ENUM (
    'active', 'paused', 'disabled'
);

CREATE TYPE execution_status AS ENUM (
    'running', 'waiting_approval', 'completed', 'failed', 'expired'
);

CREATE TYPE user_role AS ENUM (
    'admin', 'editor', 'viewer'
);

CREATE TYPE notification_channel AS ENUM (
    'slack', 'discord', 'email'
);
```

---

## Auto-Update Trigger

```sql
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

---

## Core Tables

### workspaces

```sql
CREATE TABLE workspaces (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_org_id    TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    slug            TEXT UNIQUE NOT NULL,
    tier            TEXT NOT NULL DEFAULT 'starter',
    stripe_customer_id  TEXT,
    settings        JSONB NOT NULL DEFAULT '{}',
    keyword_limit   INTEGER NOT NULL DEFAULT 3,
    mention_limit   INTEGER NOT NULL DEFAULT 50,
    seat_limit      INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER workspaces_updated_at
    BEFORE UPDATE ON workspaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### users

```sql
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_user_id   TEXT UNIQUE NOT NULL,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email           TEXT NOT NULL,
    name            TEXT NOT NULL,
    avatar_url      TEXT,
    role            user_role NOT NULL DEFAULT 'viewer',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    last_active_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_workspace ON users(workspace_id);
CREATE INDEX idx_users_clerk_id ON users(clerk_user_id);

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### keywords

```sql
CREATE TABLE keywords (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    term            TEXT NOT NULL,
    platforms       platform_type[] NOT NULL DEFAULT '{hackernews,reddit,twitter,linkedin}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    match_type      TEXT NOT NULL DEFAULT 'contains', -- 'exact', 'contains', 'regex'
    negative_terms  TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, term)
);

CREATE INDEX idx_keywords_workspace_active ON keywords(workspace_id) WHERE is_active = true;

CREATE TRIGGER keywords_updated_at
    BEFORE UPDATE ON keywords
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### mentions

```sql
CREATE TABLE mentions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id            UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    keyword_id              UUID REFERENCES keywords(id) ON DELETE SET NULL,
    platform                platform_type NOT NULL,
    platform_id             TEXT NOT NULL,
    url                     TEXT NOT NULL,
    title                   TEXT,
    content                 TEXT NOT NULL,
    content_tsv             TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    author_username         TEXT,
    author_profile_url      TEXT,
    author_karma            INTEGER,
    author_account_age_days INTEGER,
    relevance_score         REAL,
    intent                  intent_type,
    conversion_probability  REAL,
    status                  mention_status NOT NULL DEFAULT 'new',
    assigned_to             UUID REFERENCES users(id) ON DELETE SET NULL,
    platform_metadata       JSONB NOT NULL DEFAULT '{}',
    engagement_metrics      JSONB NOT NULL DEFAULT '{}',
    keyword_matches         TEXT[] NOT NULL DEFAULT '{}',
    platform_created_at     TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, platform, platform_id)
);

-- Primary query indexes
CREATE INDEX idx_mentions_workspace_status ON mentions(workspace_id, status, created_at DESC);
CREATE INDEX idx_mentions_workspace_platform ON mentions(workspace_id, platform, created_at DESC);
CREATE INDEX idx_mentions_workspace_score ON mentions(workspace_id, relevance_score DESC NULLS LAST);
CREATE INDEX idx_mentions_workspace_intent ON mentions(workspace_id, intent, created_at DESC);

-- Full-text search
CREATE INDEX idx_mentions_content_fts ON mentions USING GIN(content_tsv);

-- Partial indexes for common queries
CREATE INDEX idx_mentions_unread ON mentions(workspace_id, created_at DESC)
    WHERE status = 'new';
CREATE INDEX idx_mentions_high_intent ON mentions(workspace_id, relevance_score DESC)
    WHERE relevance_score >= 7.0 AND status = 'new';

-- Dedup lookup
CREATE INDEX idx_mentions_platform_lookup ON mentions(platform, platform_id);

CREATE TRIGGER mentions_updated_at
    BEFORE UPDATE ON mentions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### threads

```sql
CREATE TABLE threads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mention_id      UUID NOT NULL REFERENCES mentions(id) ON DELETE CASCADE,
    platform        platform_type NOT NULL,
    thread_id       TEXT NOT NULL,
    content         JSONB NOT NULL DEFAULT '[]', -- Array of thread messages
    summary         TEXT,
    sentiment       TEXT, -- 'positive', 'negative', 'neutral', 'hostile'
    solutions_mentioned TEXT[] NOT NULL DEFAULT '{}',
    our_product_mentioned BOOLEAN NOT NULL DEFAULT false,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_threads_mention ON threads(mention_id);
```

### replies

```sql
CREATE TABLE replies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mention_id      UUID NOT NULL REFERENCES mentions(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    variant         reply_variant NOT NULL,
    content         TEXT NOT NULL,
    edited_content  TEXT, -- User-edited version (null if not edited)
    status          reply_status NOT NULL DEFAULT 'draft',
    platform_post_id TEXT, -- ID of the post on the platform after posting
    posted_by       UUID REFERENCES users(id),
    approved_by     UUID REFERENCES users(id),
    utm_link_id     UUID REFERENCES utm_links(id),
    safe_link_score REAL, -- Score from safe link rules engine
    safe_link_flags JSONB NOT NULL DEFAULT '{}',
    posted_at       TIMESTAMPTZ,
    approved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_replies_mention ON replies(mention_id);
CREATE INDEX idx_replies_workspace_status ON replies(workspace_id, status, created_at DESC);

CREATE TRIGGER replies_updated_at
    BEFORE UPDATE ON replies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### leads

```sql
CREATE TABLE leads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    mention_id      UUID REFERENCES mentions(id) ON DELETE SET NULL,
    stage           lead_stage NOT NULL DEFAULT 'prospect',
    contact_name    TEXT,
    contact_email   TEXT,
    company         TEXT,
    username        TEXT,
    platform        platform_type,
    profile_url     TEXT,
    estimated_value INTEGER DEFAULT 0, -- in cents
    notes           TEXT,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_leads_workspace_stage ON leads(workspace_id, stage, created_at DESC);
CREATE INDEX idx_leads_workspace_value ON leads(workspace_id, estimated_value DESC);

CREATE TRIGGER leads_updated_at
    BEFORE UPDATE ON leads
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### lead_events

```sql
CREATE TABLE lead_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id         UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    previous_stage  lead_stage,
    new_stage       lead_stage NOT NULL,
    changed_by      UUID REFERENCES users(id),
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lead_events_lead ON lead_events(lead_id, created_at DESC);
```

### utm_links

```sql
CREATE TABLE utm_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code            TEXT UNIQUE NOT NULL, -- short code for redirect URL
    destination_url TEXT NOT NULL,
    utm_source      TEXT NOT NULL, -- platform name
    utm_medium      TEXT NOT NULL DEFAULT 'social_reply',
    utm_campaign    TEXT,
    utm_content     TEXT, -- variant type
    click_count     INTEGER NOT NULL DEFAULT 0,
    signup_count    INTEGER NOT NULL DEFAULT 0,
    revenue_cents   INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_utm_links_workspace ON utm_links(workspace_id, created_at DESC);
CREATE INDEX idx_utm_links_code ON utm_links(code);
```

### utm_events

```sql
CREATE TABLE utm_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    utm_link_id     UUID NOT NULL REFERENCES utm_links(id) ON DELETE CASCADE,
    event_type      TEXT NOT NULL, -- 'click', 'signup', 'purchase'
    referrer        TEXT,
    user_agent      TEXT,
    ip_hash         TEXT, -- hashed IP for privacy
    revenue_cents   INTEGER DEFAULT 0,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_utm_events_link ON utm_events(utm_link_id, created_at DESC);
CREATE INDEX idx_utm_events_type ON utm_events(event_type, created_at DESC);
```

### documents (RAG Knowledge Base)

```sql
CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    content_type    TEXT NOT NULL DEFAULT 'markdown', -- 'markdown', 'pdf', 'text', 'url'
    source_url      TEXT,
    file_size_bytes INTEGER,
    chunk_count     INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_documents_workspace ON documents(workspace_id) WHERE is_active = true;

CREATE TRIGGER documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### document_chunks (pgvector)

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE document_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    content_tsv     TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    embedding       vector(1024) NOT NULL, -- Voyage AI voyage-3
    chunk_index     INTEGER NOT NULL,
    section_title   TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    token_count     INTEGER,
    is_exemplar     BOOLEAN NOT NULL DEFAULT false, -- High-performing reply examples
    effectiveness_score REAL DEFAULT 0.0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- HNSW index for fast vector similarity search
CREATE INDEX idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Full-text search index
CREATE INDEX idx_chunks_content_fts ON document_chunks USING GIN(content_tsv);

-- Workspace filter index
CREATE INDEX idx_chunks_workspace ON document_chunks(workspace_id, document_id);

-- Exemplar lookup
CREATE INDEX idx_chunks_exemplars ON document_chunks(workspace_id)
    WHERE is_exemplar = true;
```

### workflows

```sql
CREATE TABLE workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    status          workflow_status NOT NULL DEFAULT 'active',
    trigger_config  JSONB NOT NULL, -- {platform, min_score, intent_types, keywords}
    action_chain    JSONB NOT NULL, -- [{type, config}, ...]
    max_triggers_per_hour INTEGER NOT NULL DEFAULT 10,
    execution_count INTEGER NOT NULL DEFAULT 0,
    last_triggered_at TIMESTAMPTZ,
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflows_workspace ON workflows(workspace_id) WHERE status = 'active';

CREATE TRIGGER workflows_updated_at
    BEFORE UPDATE ON workflows
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### workflow_executions

```sql
CREATE TABLE workflow_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    mention_id      UUID REFERENCES mentions(id) ON DELETE SET NULL,
    status          execution_status NOT NULL DEFAULT 'running',
    steps           JSONB NOT NULL DEFAULT '[]', -- [{action, status, started_at, completed_at, output}]
    current_step    INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_executions_workflow ON workflow_executions(workflow_id, created_at DESC);
CREATE INDEX idx_executions_status ON workflow_executions(status) WHERE status IN ('running', 'waiting_approval');
```

### platform_accounts

```sql
CREATE TABLE platform_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform        platform_type NOT NULL,
    username        TEXT,
    platform_user_id TEXT,
    access_token_enc TEXT, -- AES-256-GCM encrypted
    refresh_token_enc TEXT, -- AES-256-GCM encrypted
    token_expires_at TIMESTAMPTZ,
    karma           INTEGER,
    account_age_days INTEGER,
    trust_score     REAL DEFAULT 0.0,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, user_id, platform)
);

CREATE INDEX idx_platform_accounts_workspace ON platform_accounts(workspace_id, platform);

CREATE TRIGGER platform_accounts_updated_at
    BEFORE UPDATE ON platform_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

### notifications

```sql
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel         notification_channel NOT NULL,
    recipient       TEXT NOT NULL, -- email, Slack channel, Discord webhook URL
    subject         TEXT,
    body            TEXT NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    sent_at         TIMESTAMPTZ,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_workspace ON notifications(workspace_id, created_at DESC);
```

### subscription_plans

```sql
CREATE TABLE subscription_plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL, -- 'starter', 'solo', 'growth', 'scale'
    stripe_price_id TEXT UNIQUE,
    price_cents     INTEGER NOT NULL,
    keyword_limit   INTEGER NOT NULL,
    mention_limit   INTEGER NOT NULL,
    seat_limit      INTEGER NOT NULL,
    features        JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### subscriptions

```sql
CREATE TABLE subscriptions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    plan_id             UUID NOT NULL REFERENCES subscription_plans(id),
    stripe_subscription_id TEXT UNIQUE,
    status              TEXT NOT NULL DEFAULT 'active', -- 'active', 'past_due', 'cancelled', 'trialing'
    current_period_start TIMESTAMPTZ,
    current_period_end  TIMESTAMPTZ,
    cancel_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_subscriptions_workspace ON subscriptions(workspace_id);

CREATE TRIGGER subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

---

## Materialized Views

### mv_mention_stats_daily

```sql
CREATE MATERIALIZED VIEW mv_mention_stats_daily AS
SELECT
    workspace_id,
    platform,
    DATE_TRUNC('day', created_at) AS day,
    COUNT(*) AS total_mentions,
    COUNT(*) FILTER (WHERE status = 'replied') AS replied_count,
    COUNT(*) FILTER (WHERE intent = 'buy_signal') AS buy_signals,
    AVG(relevance_score) AS avg_relevance,
    COUNT(*) FILTER (WHERE relevance_score >= 7.0) AS high_intent_count
FROM mentions
GROUP BY workspace_id, platform, DATE_TRUNC('day', created_at);

CREATE UNIQUE INDEX idx_mv_mention_stats_daily
    ON mv_mention_stats_daily(workspace_id, platform, day);
```

### mv_lead_funnel

```sql
CREATE MATERIALIZED VIEW mv_lead_funnel AS
SELECT
    workspace_id,
    stage,
    COUNT(*) AS lead_count,
    SUM(estimated_value) AS total_value,
    AVG(estimated_value) AS avg_value
FROM leads
GROUP BY workspace_id, stage;

CREATE UNIQUE INDEX idx_mv_lead_funnel
    ON mv_lead_funnel(workspace_id, stage);
```

### mv_reply_performance

```sql
CREATE MATERIALIZED VIEW mv_reply_performance AS
SELECT
    r.workspace_id,
    m.platform,
    r.variant,
    COUNT(*) AS total_replies,
    COUNT(*) FILTER (WHERE r.status = 'posted') AS posted_count,
    COUNT(ul.id) FILTER (WHERE ul.click_count > 0) AS clicked_count,
    COUNT(ul.id) FILTER (WHERE ul.signup_count > 0) AS converted_count,
    SUM(ul.click_count) AS total_clicks,
    SUM(ul.signup_count) AS total_signups
FROM replies r
JOIN mentions m ON r.mention_id = m.id
LEFT JOIN utm_links ul ON r.utm_link_id = ul.id
GROUP BY r.workspace_id, m.platform, r.variant;

CREATE UNIQUE INDEX idx_mv_reply_performance
    ON mv_reply_performance(workspace_id, platform, variant);
```

### mv_keyword_performance

```sql
CREATE MATERIALIZED VIEW mv_keyword_performance AS
SELECT
    k.workspace_id,
    k.id AS keyword_id,
    k.term,
    COUNT(m.id) AS mention_count,
    COUNT(r.id) FILTER (WHERE r.status = 'posted') AS reply_count,
    COUNT(l.id) AS lead_count,
    COUNT(l.id) FILTER (WHERE l.stage = 'converted') AS conversion_count,
    AVG(m.relevance_score) AS avg_relevance
FROM keywords k
LEFT JOIN mentions m ON m.keyword_id = k.id
LEFT JOIN replies r ON r.mention_id = m.id
LEFT JOIN leads l ON l.mention_id = m.id
GROUP BY k.workspace_id, k.id, k.term;

CREATE UNIQUE INDEX idx_mv_keyword_performance
    ON mv_keyword_performance(workspace_id, keyword_id);
```

### Refresh Schedule

```sql
-- Refresh all materialized views (run via cron every 15 minutes)
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_mention_stats_daily;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_lead_funnel;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_reply_performance;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_keyword_performance;
```

---

## Example sqlc Queries

```sql
-- queries/mentions.sql

-- name: ListMentionsByWorkspace :many
SELECT id, platform, content, url, author_username, relevance_score,
       intent, status, keyword_matches, created_at
FROM mentions
WHERE workspace_id = $1
  AND ($2::mention_status IS NULL OR status = $2)
  AND ($3::platform_type IS NULL OR platform = $3)
  AND ($4::real IS NULL OR relevance_score >= $4)
ORDER BY created_at DESC
LIMIT $5 OFFSET $6;

-- name: GetMentionWithThread :one
SELECT m.*, t.content AS thread_content, t.summary AS thread_summary,
       t.sentiment, t.solutions_mentioned, t.our_product_mentioned
FROM mentions m
LEFT JOIN threads t ON t.mention_id = m.id
WHERE m.id = $1 AND m.workspace_id = $2;

-- name: InsertMention :one
INSERT INTO mentions (
    workspace_id, keyword_id, platform, platform_id, url, title,
    content, author_username, author_profile_url, keyword_matches,
    platform_metadata, engagement_metrics, platform_created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (workspace_id, platform, platform_id) DO NOTHING
RETURNING *;

-- name: UpdateMentionScore :exec
UPDATE mentions
SET relevance_score = $3, intent = $4, conversion_probability = $5
WHERE id = $1 AND workspace_id = $2;

-- name: SearchMentionsByVector :many
SELECT dc.content, dc.section_title, dc.document_id,
       1 - (dc.embedding <=> $1::vector) AS similarity
FROM document_chunks dc
WHERE dc.workspace_id = $2
ORDER BY dc.embedding <=> $1::vector
LIMIT $3;

-- name: GetWorkspaceStats :one
SELECT
    COUNT(*) FILTER (WHERE status = 'new') AS unread_count,
    COUNT(*) FILTER (WHERE relevance_score >= 7.0 AND status = 'new') AS high_intent_count,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours') AS today_count
FROM mentions
WHERE workspace_id = $1;
```

---

## Migration Strategy

### Naming Convention

```
migrations/
├── 00001_init_schema.sql
├── 00002_add_pgvector.sql
├── 00003_add_materialized_views.sql
├── 00004_seed_subscription_plans.sql
└── ...
```

### Migration Template

```sql
-- +goose Up
CREATE TABLE example (...);

-- +goose Down
DROP TABLE IF EXISTS example;
```

### Workflow

1. Create migration: `goose -dir migrations create add_feature sql`
2. Write UP and DOWN SQL
3. Apply: `goose -dir migrations postgres "$DATABASE_URL" up`
4. Generate Go code: `sqlc generate`
5. Test: `go test ./internal/database/...`
6. Commit migration + generated code together

### Rollback

- Always write reversible `-- +goose Down` blocks
- Test rollback locally before pushing
- In production: `goose -dir migrations postgres "$DATABASE_URL" down`
- Never delete applied migrations, only add new ones
