-- +goose Up
-- billing: plans + prepaid credits. Payments run through Dodo Payments; the
-- balance lives here so every metered action is checked instantly.
ALTER TABLE workspaces
    ADD COLUMN plan TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'starter', 'pro')),
    ADD COLUMN credit_balance INTEGER NOT NULL DEFAULT 300 CHECK (credit_balance >= 0),
    ADD COLUMN credits_refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN plan_renews_at TIMESTAMPTZ,
    ADD COLUMN dodo_customer_id TEXT,
    ADD COLUMN dodo_subscription_id TEXT;

UPDATE workspaces SET keyword_limit = 3 WHERE plan = 'free';

-- Grants (plan allowances, top-ups, manual adjustments). ref makes a grant
-- idempotent: a retried webhook for the same period or payment is a no-op.
CREATE TABLE credit_ledger (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    delta         INTEGER NOT NULL,
    balance_after INTEGER NOT NULL,
    reason        TEXT NOT NULL,
    ref           TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, ref)
);
CREATE INDEX credit_ledger_workspace_created ON credit_ledger (workspace_id, created_at DESC);

-- Spending is aggregated per day and reason instead of one row per post.
CREATE TABLE credit_usage_daily (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    day          DATE NOT NULL,
    reason       TEXT NOT NULL,
    credits      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (workspace_id, day, reason)
);

-- +goose Down
DROP TABLE credit_usage_daily;
DROP TABLE credit_ledger;
ALTER TABLE workspaces
    DROP COLUMN dodo_subscription_id,
    DROP COLUMN dodo_customer_id,
    DROP COLUMN plan_renews_at,
    DROP COLUMN credits_refreshed_at,
    DROP COLUMN credit_balance,
    DROP COLUMN plan;
