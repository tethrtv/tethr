-- +goose Up
ALTER TABLE watch_events ADD COLUMN precision text NOT NULL DEFAULT 'time' CHECK (precision IN ('time', 'day', 'none'));

-- +goose Down
ALTER TABLE watch_events DROP COLUMN precision;
