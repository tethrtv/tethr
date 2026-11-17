-- +goose Up
ALTER TABLE lists ADD COLUMN rule jsonb;
CREATE INDEX titles_studios ON titles USING gin (studios) WHERE merged_into_id IS NULL;
CREATE INDEX titles_networks ON titles USING gin (networks) WHERE merged_into_id IS NULL;

-- +goose Down
DROP INDEX titles_networks;
DROP INDEX titles_studios;
ALTER TABLE lists DROP COLUMN rule;
