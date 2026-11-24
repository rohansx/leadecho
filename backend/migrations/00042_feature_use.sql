-- +goose Up
-- Demand grouping and competitor watch now run only for workspaces that
-- opened the page in the last 14 days (settings.feature_use). Starting every
-- workspace at "used now" gives today's users 14 days to open the page
-- before their jobs pause; opening it again resumes them.
UPDATE workspaces
SET settings = COALESCE(settings, '{}'::jsonb) || jsonb_build_object('feature_use',
        COALESCE(settings->'feature_use', '{}'::jsonb) || jsonb_build_object('demand', now(), 'competitors', now()));

-- +goose Down
UPDATE workspaces SET settings = settings - 'feature_use';
