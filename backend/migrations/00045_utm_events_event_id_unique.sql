-- +goose NO TRANSACTION

-- +goose Up
-- One conversion per caller-supplied event_id per tracked link: a retried
-- sign-up webhook hits the unique violation and is answered "duplicate"
-- instead of counting twice (attribution.RecordConversion). CONCURRENTLY keeps
-- utm_events writable while it builds; the DROP first lets a failed build retry.
DROP INDEX CONCURRENTLY IF EXISTS utm_events_link_event_id_unique;
CREATE UNIQUE INDEX CONCURRENTLY utm_events_link_event_id_unique
    ON utm_events (utm_link_id, (metadata->>'event_id')) WHERE metadata ? 'event_id';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS utm_events_link_event_id_unique;
