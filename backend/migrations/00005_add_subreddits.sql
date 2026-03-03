-- +goose Up
ALTER TABLE keywords ADD COLUMN subreddits TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE keywords DROP COLUMN subreddits;
