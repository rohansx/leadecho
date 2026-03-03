-- +goose NO TRANSACTION

-- +goose Up
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'devto';
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'lobsters';
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'indiehackers';

-- +goose Down
-- PostgreSQL cannot remove enum values; this is a no-op.
