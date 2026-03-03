-- +goose Up

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

-- Enums
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

-- Auto-update trigger function
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- ─── Core Tables ───────────────────────────────────────

-- workspaces
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

-- users
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

-- keywords
CREATE TABLE keywords (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    term            TEXT NOT NULL,
    platforms       platform_type[] NOT NULL DEFAULT '{hackernews,reddit,twitter,linkedin}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    match_type      TEXT NOT NULL DEFAULT 'contains',
    negative_terms  TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, term)
);

CREATE INDEX idx_keywords_workspace_active ON keywords(workspace_id) WHERE is_active = true;

CREATE TRIGGER keywords_updated_at
    BEFORE UPDATE ON keywords
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- mentions
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

CREATE INDEX idx_mentions_workspace_status ON mentions(workspace_id, status, created_at DESC);
CREATE INDEX idx_mentions_workspace_platform ON mentions(workspace_id, platform, created_at DESC);
CREATE INDEX idx_mentions_workspace_score ON mentions(workspace_id, relevance_score DESC NULLS LAST);
CREATE INDEX idx_mentions_workspace_intent ON mentions(workspace_id, intent, created_at DESC);
CREATE INDEX idx_mentions_content_fts ON mentions USING GIN(content_tsv);
CREATE INDEX idx_mentions_unread ON mentions(workspace_id, created_at DESC)
    WHERE status = 'new';
CREATE INDEX idx_mentions_high_intent ON mentions(workspace_id, relevance_score DESC)
    WHERE relevance_score >= 7.0 AND status = 'new';
CREATE INDEX idx_mentions_platform_lookup ON mentions(platform, platform_id);

CREATE TRIGGER mentions_updated_at
    BEFORE UPDATE ON mentions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- threads
CREATE TABLE threads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mention_id      UUID NOT NULL REFERENCES mentions(id) ON DELETE CASCADE,
    platform        platform_type NOT NULL,
    thread_id       TEXT NOT NULL,
    content         JSONB NOT NULL DEFAULT '[]',
    summary         TEXT,
    sentiment       TEXT,
    solutions_mentioned TEXT[] NOT NULL DEFAULT '{}',
    our_product_mentioned BOOLEAN NOT NULL DEFAULT false,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_threads_mention ON threads(mention_id);

-- utm_links (created before replies so the FK works)
CREATE TABLE utm_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code            TEXT UNIQUE NOT NULL,
    destination_url TEXT NOT NULL,
    utm_source      TEXT NOT NULL,
    utm_medium      TEXT NOT NULL DEFAULT 'social_reply',
    utm_campaign    TEXT,
    utm_content     TEXT,
    click_count     INTEGER NOT NULL DEFAULT 0,
    signup_count    INTEGER NOT NULL DEFAULT 0,
    revenue_cents   INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_utm_links_workspace ON utm_links(workspace_id, created_at DESC);
CREATE INDEX idx_utm_links_code ON utm_links(code);

-- replies
CREATE TABLE replies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mention_id      UUID NOT NULL REFERENCES mentions(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    variant         reply_variant NOT NULL,
    content         TEXT NOT NULL,
    edited_content  TEXT,
    status          reply_status NOT NULL DEFAULT 'draft',
    platform_post_id TEXT,
    posted_by       UUID REFERENCES users(id),
    approved_by     UUID REFERENCES users(id),
    utm_link_id     UUID REFERENCES utm_links(id),
    safe_link_score REAL,
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

-- leads
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
    estimated_value INTEGER DEFAULT 0,
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

-- lead_events
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

-- utm_events
CREATE TABLE utm_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    utm_link_id     UUID NOT NULL REFERENCES utm_links(id) ON DELETE CASCADE,
    event_type      TEXT NOT NULL,
    referrer        TEXT,
    user_agent      TEXT,
    ip_hash         TEXT,
    revenue_cents   INTEGER DEFAULT 0,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_utm_events_link ON utm_events(utm_link_id, created_at DESC);
CREATE INDEX idx_utm_events_type ON utm_events(event_type, created_at DESC);

-- ─── Knowledge Base (RAG) ──────────────────────────────

-- documents
CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    content_type    TEXT NOT NULL DEFAULT 'markdown',
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

-- document_chunks (pgvector)
CREATE TABLE document_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    content_tsv     TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    embedding       vector(1024) NOT NULL,
    chunk_index     INTEGER NOT NULL,
    section_title   TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    token_count     INTEGER,
    is_exemplar     BOOLEAN NOT NULL DEFAULT false,
    effectiveness_score REAL DEFAULT 0.0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
CREATE INDEX idx_chunks_content_fts ON document_chunks USING GIN(content_tsv);
CREATE INDEX idx_chunks_workspace ON document_chunks(workspace_id, document_id);
CREATE INDEX idx_chunks_exemplars ON document_chunks(workspace_id)
    WHERE is_exemplar = true;

-- ─── Workflows ─────────────────────────────────────────

-- workflows
CREATE TABLE workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    status          workflow_status NOT NULL DEFAULT 'active',
    trigger_config  JSONB NOT NULL,
    action_chain    JSONB NOT NULL,
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

-- workflow_executions
CREATE TABLE workflow_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    mention_id      UUID REFERENCES mentions(id) ON DELETE SET NULL,
    status          execution_status NOT NULL DEFAULT 'running',
    steps           JSONB NOT NULL DEFAULT '[]',
    current_step    INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_executions_workflow ON workflow_executions(workflow_id, created_at DESC);
CREATE INDEX idx_executions_status ON workflow_executions(status) WHERE status IN ('running', 'waiting_approval');

-- ─── Platform & Billing ────────────────────────────────

-- platform_accounts
CREATE TABLE platform_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform        platform_type NOT NULL,
    username        TEXT,
    platform_user_id TEXT,
    access_token_enc TEXT,
    refresh_token_enc TEXT,
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

-- notifications
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel         notification_channel NOT NULL,
    recipient       TEXT NOT NULL,
    subject         TEXT,
    body            TEXT NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    sent_at         TIMESTAMPTZ,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_workspace ON notifications(workspace_id, created_at DESC);

-- subscription_plans
CREATE TABLE subscription_plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    stripe_price_id TEXT UNIQUE,
    price_cents     INTEGER NOT NULL,
    keyword_limit   INTEGER NOT NULL,
    mention_limit   INTEGER NOT NULL,
    seat_limit      INTEGER NOT NULL,
    features        JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- subscriptions
CREATE TABLE subscriptions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    plan_id             UUID NOT NULL REFERENCES subscription_plans(id),
    stripe_subscription_id TEXT UNIQUE,
    status              TEXT NOT NULL DEFAULT 'active',
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


-- +goose Down

DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS platform_accounts;
DROP TABLE IF EXISTS workflow_executions;
DROP TABLE IF EXISTS workflows;
DROP TABLE IF EXISTS document_chunks;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS utm_events;
DROP TABLE IF EXISTS lead_events;
DROP TABLE IF EXISTS leads;
DROP TABLE IF EXISTS replies;
DROP TABLE IF EXISTS utm_links;
DROP TABLE IF EXISTS threads;
DROP TABLE IF EXISTS mentions;
DROP TABLE IF EXISTS keywords;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS workspaces;
DROP FUNCTION IF EXISTS update_updated_at;
DROP TYPE IF EXISTS notification_channel;
DROP TYPE IF EXISTS user_role;
DROP TYPE IF EXISTS execution_status;
DROP TYPE IF EXISTS workflow_status;
DROP TYPE IF EXISTS reply_variant;
DROP TYPE IF EXISTS reply_status;
DROP TYPE IF EXISTS lead_stage;
DROP TYPE IF EXISTS intent_type;
DROP TYPE IF EXISTS mention_status;
DROP TYPE IF EXISTS platform_type;
DROP EXTENSION IF EXISTS vector;
DROP EXTENSION IF EXISTS "uuid-ossp";
