-- +goose Up

-- Link keywords to monitoring profiles (added to support multi-profile workspaces).
-- profile_id is required: every keyword must belong to a monitoring profile so the
-- monitor knows which pain-point embeddings to score against.
ALTER TABLE keywords ADD COLUMN IF NOT EXISTS profile_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Ensure every workspace with keywords has at least one active profile to
-- backfill into (fresh/dev-seeded workspaces may have keywords but no
-- profile yet).
INSERT INTO monitoring_profiles (workspace_id, name, description)
SELECT DISTINCT k.workspace_id, 'Uncategorized', 'Auto-created during profile_id backfill'
FROM keywords k
WHERE NOT EXISTS (
    SELECT 1 FROM monitoring_profiles mp
    WHERE mp.workspace_id = k.workspace_id AND mp.is_active = true
);

-- Backfill: assign existing keywords to their workspace's first active profile.
UPDATE keywords k
SET profile_id = sub.profile_id
FROM (
    SELECT k2.workspace_id, MIN(mp.id::text)::uuid AS profile_id
    FROM keywords k2
    JOIN monitoring_profiles mp ON mp.workspace_id = k2.workspace_id AND mp.is_active = true
    GROUP BY k2.workspace_id
) sub
WHERE k.workspace_id = sub.workspace_id
  AND k.profile_id = '00000000-0000-0000-0000-000000000000';

-- Add FK + index (after backfill so existing rows satisfy the constraint).
ALTER TABLE keywords
    DROP CONSTRAINT IF EXISTS keywords_profile_id_fkey,
    ADD CONSTRAINT keywords_profile_id_fkey
        FOREIGN KEY (profile_id) REFERENCES monitoring_profiles(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_keywords_profile ON keywords(profile_id) WHERE is_active = true;

-- +goose Down

ALTER TABLE keywords DROP CONSTRAINT IF EXISTS keywords_profile_id_fkey;
DROP INDEX IF EXISTS idx_keywords_profile;
ALTER TABLE keywords DROP COLUMN IF EXISTS profile_id;
