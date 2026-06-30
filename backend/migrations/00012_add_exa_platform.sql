-- +goose NO TRANSACTION

-- +goose Up
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'exa';

-- +goose Down
-- PostgreSQL cannot remove enum values; this is a no-op.
