-- +goose Up
-- The Keycloak user made for a temp (claimable) account, so a claim deletes
-- exactly that user and never another one that shares the email.
ALTER TABLE users ADD COLUMN idp_user_id TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS idp_user_id;
