-- +goose Up
ALTER TABLE users ADD COLUMN password_hash TEXT;
CREATE UNIQUE INDEX idx_users_email ON users(email);

-- +goose Down
DROP INDEX IF EXISTS idx_users_email;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
