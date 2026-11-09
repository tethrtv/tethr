-- +goose Up
ALTER TABLE profiles ADD COLUMN in_popular boolean NOT NULL DEFAULT true;
CREATE INDEX watch_events_recent ON watch_events (watched_at);

-- +goose Down
DROP INDEX watch_events_recent;
ALTER TABLE profiles DROP COLUMN in_popular;
