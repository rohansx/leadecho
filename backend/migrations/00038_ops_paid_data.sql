-- +goose Up
-- ops: owner-managed access to paid data (treg) and scanning.
ALTER TABLE workspaces
    ADD COLUMN paid_data BOOLEAN NOT NULL DEFAULT false,      -- may spend the operator's treg balance
    ADD COLUMN scanning_paused BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN plan_source TEXT NOT NULL DEFAULT 'free' CHECK (plan_source IN ('free', 'dodo', 'comp'));

-- Paying subscribers already on a paid plan keep paid data.
UPDATE workspaces SET paid_data = true, plan_source = 'dodo' WHERE plan IN ('starter', 'pro') AND dodo_subscription_id IS NOT NULL;

-- Global switches (one row per key).
CREATE TABLE ops_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO ops_settings (key, value) VALUES ('paid_data_paused', 'false');

-- Every /ops action, who did it, and what changed.
CREATE TABLE ops_audit (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_email  TEXT NOT NULL,
    action       TEXT NOT NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    detail       JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ops_audit_created ON ops_audit (created_at DESC);

-- +goose Down
DROP TABLE ops_audit;
DROP TABLE ops_settings;
ALTER TABLE workspaces DROP COLUMN plan_source, DROP COLUMN scanning_paused, DROP COLUMN paid_data;
