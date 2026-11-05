-- +goose Up
-- automations (P2 #17): scheduled deterministic playbooks + run log.
CREATE TABLE automations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('daily_triage', 'lead_sweep', 'attribution_review', 'custom')),
    frequency    TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly')),
    run_time     TEXT NOT NULL DEFAULT '09:30', -- HH:MM, local to timezone
    weekday      TEXT NOT NULL DEFAULT 'monday', -- weekly only
    timezone     TEXT NOT NULL DEFAULT 'UTC',
    enabled      BOOLEAN NOT NULL DEFAULT false,
    config       JSONB NOT NULL DEFAULT '{}',
    last_run_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_automations_workspace ON automations (workspace_id, created_at);
CREATE INDEX idx_automations_enabled ON automations (id) WHERE enabled;

CREATE TABLE automation_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    automation_id UUID NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    trigger       TEXT NOT NULL CHECK (trigger IN ('schedule', 'manual')),
    -- scheduled runs own one period; manual runs have NULL (never conflict)
    period_start  TIMESTAMPTZ,
    status        TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
    output        JSONB NOT NULL DEFAULT '{}',
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,
    UNIQUE (automation_id, period_start)
);
CREATE INDEX idx_automation_runs_automation ON automation_runs (automation_id, started_at DESC);

-- In-app notifications reuse the notifications table.
ALTER TYPE notification_channel ADD VALUE IF NOT EXISTS 'in_app';

-- +goose Down
-- ponytail: enum values cannot be dropped; 'in_app' stays on notification_channel.
DROP TABLE IF EXISTS automation_runs;
DROP TABLE IF EXISTS automations;
