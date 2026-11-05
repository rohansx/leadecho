-- +goose Up
-- Plain-English intent per listener (monitoring profile): what it looks for,
-- what it skips, and the onboarding goal it was built for. Shared by the
-- onboarding listener builder (writes) and the scorer (reads).
ALTER TABLE monitoring_profiles ADD COLUMN looks_for TEXT NOT NULL DEFAULT '';
ALTER TABLE monitoring_profiles ADD COLUMN skips TEXT NOT NULL DEFAULT '';
ALTER TABLE monitoring_profiles ADD COLUMN goal TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE monitoring_profiles DROP COLUMN goal;
ALTER TABLE monitoring_profiles DROP COLUMN skips;
ALTER TABLE monitoring_profiles DROP COLUMN looks_for;
