-- +goose Up
-- Digests (P1 #13): per-workspace daily/weekly digest schedule + idempotent run log.
-- A workspace with no row gets the defaults below (digests are on by default).
CREATE TABLE digest_settings (
    workspace_id   UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    daily_enabled  BOOLEAN NOT NULL DEFAULT true,
    daily_time     TEXT NOT NULL DEFAULT '09:30',
    weekly_enabled BOOLEAN NOT NULL DEFAULT true,
    weekly_day     TEXT NOT NULL DEFAULT 'monday',
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    -- empty = the workspace owner's email, resolved at send time
    recipients     TEXT[] NOT NULL DEFAULT '{}',
    channels       TEXT[] NOT NULL DEFAULT '{email}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE digest_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('daily', 'weekly')),
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'skipped', 'failed')),
    attempts     INT NOT NULL DEFAULT 1,
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, kind, period_start)
);

-- +goose Down
DROP TABLE IF EXISTS digest_runs;
DROP TABLE IF EXISTS digest_settings;
