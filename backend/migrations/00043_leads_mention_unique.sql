-- +goose NO TRANSACTION

-- +goose Up
-- One leads row per post: two scorings of the same post (a re-score racing a
-- scan, the async qualifier and the inline one) inserted two leads, each with
-- its own enrichment and CRM event. qualifyAsLead treats the unique violation
-- as "already a lead". CONCURRENTLY keeps leads writable while it builds, so
-- it cannot run in a transaction. Production had no duplicates when written;
-- if this fails on one, an INVALID index is left: drop it, merge the
-- duplicates, migrate again.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS leads_mention_id_unique
    ON leads (mention_id) WHERE mention_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS leads_mention_id_unique;
