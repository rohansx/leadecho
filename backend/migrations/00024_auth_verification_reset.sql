-- +goose Up
-- Built-in auth: email verification, password reset, session revocation.

ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN session_version INT NOT NULL DEFAULT 0;

-- Existing OAuth users proved their email with the provider.
UPDATE users SET email_verified = true
WHERE clerk_user_id LIKE 'google_%' OR clerk_user_id LIKE 'github_%';

CREATE TABLE auth_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('verify', 'reset')),
    token_hash  TEXT NOT NULL UNIQUE, -- sha256 hex; the raw token is never stored
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_tokens_user_kind ON auth_tokens(user_id, kind) WHERE used_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth_tokens;
ALTER TABLE users DROP COLUMN IF EXISTS session_version;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
