-- +goose Up
-- The kind of product a workspace sells, which picks the lead facts the
-- classifier pulls from a post (ai/facts.go). '' = no facts, as before.
ALTER TABLE brand_profiles ADD COLUMN archetype TEXT NOT NULL DEFAULT ''
    CHECK (archetype IN ('', 'devtool', 'hiring', 'b2b_saas', 'vertical_smb', 'prosumer'));

-- +goose Down
ALTER TABLE brand_profiles DROP COLUMN archetype;
