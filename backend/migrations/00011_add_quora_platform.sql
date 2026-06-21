-- +goose NO TRANSACTION

-- +goose Up
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'quora';

-- +goose Down
-- PostgreSQL cannot remove enum values; this is a no-op.
