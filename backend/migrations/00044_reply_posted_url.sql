-- +goose Up
-- The link to the comment the customer posted (LeadEcho never posts): pasted
-- when they mark a reply posted, so "Replies that paid off" can open it and an
-- engagement check can find it later. Nullable: older posted replies have none.
ALTER TABLE replies ADD COLUMN IF NOT EXISTS posted_url TEXT;

-- +goose Down
ALTER TABLE replies DROP COLUMN IF EXISTS posted_url;
