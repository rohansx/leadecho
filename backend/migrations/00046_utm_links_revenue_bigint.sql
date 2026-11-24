-- +goose Up
-- A link's running revenue total could pass int32 (~$21.4M in cents) and make
-- every later conversion on it fail. Per-event revenue stays INTEGER (capped
-- at 10,000,000 by the conversion API). utm_links is small: the rewrite is brief.
ALTER TABLE utm_links ALTER COLUMN revenue_cents TYPE BIGINT;

-- +goose Down
ALTER TABLE utm_links ALTER COLUMN revenue_cents TYPE INTEGER;
