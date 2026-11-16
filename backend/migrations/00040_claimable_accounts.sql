-- +goose Up
-- claimable accounts: an operator builds an account for a prospect under a
-- placeholder email; the prospect takes it over through a Lobby claim link,
-- which swaps in their verified email and clears this flag.
ALTER TABLE users ADD COLUMN claimable BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX idx_users_claimable ON users(created_at DESC) WHERE claimable;

-- +goose Down
DROP INDEX IF EXISTS idx_users_claimable;
ALTER TABLE users DROP COLUMN IF EXISTS claimable;
