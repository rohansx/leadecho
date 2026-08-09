-- +goose Up
-- Google News as a monitoring source. Read via the public RSS search feed, so
-- it needs no API key or browser sidecar.
--
-- NOTE: Google's feed carries a copyright notice restricting it to "a personal
-- feed reader for personal, non-commercial use". The crawler is therefore gated
-- behind GOOGLE_NEWS_ENABLED (default false) so it is opt-in for self-hosters
-- rather than on by default in any distributed build.
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'googlenews';

-- +goose Down
-- Postgres cannot remove a value from an enum type. Rows using 'googlenews'
-- would have to be migrated to another platform and the type rebuilt, so this
-- is intentionally a no-op rather than a destructive rewrite.
SELECT 1;
