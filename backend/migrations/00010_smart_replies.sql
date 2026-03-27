-- +goose Up

-- Awareness level classification for mentions (SubredditSignals-style 4-stage taxonomy).
ALTER TABLE mentions ADD COLUMN awareness_level TEXT;

-- Reply template variant tracking.
ALTER TABLE replies ADD COLUMN template_style TEXT;
ALTER TABLE replies ADD COLUMN thread_context_used BOOLEAN NOT NULL DEFAULT false;

-- Reply engagement tracking (post-reply metrics).
CREATE TABLE reply_engagements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reply_id        UUID NOT NULL REFERENCES replies(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    upvotes         INTEGER NOT NULL DEFAULT 0,
    downvotes       INTEGER NOT NULL DEFAULT 0,
    reply_count     INTEGER NOT NULL DEFAULT 0,
    is_removed      BOOLEAN NOT NULL DEFAULT false,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reply_engagements_reply ON reply_engagements(reply_id, checked_at DESC);

-- +goose Down
DROP TABLE IF EXISTS reply_engagements;
ALTER TABLE replies DROP COLUMN IF EXISTS thread_context_used;
ALTER TABLE replies DROP COLUMN IF EXISTS template_style;
ALTER TABLE mentions DROP COLUMN IF EXISTS awareness_level;
