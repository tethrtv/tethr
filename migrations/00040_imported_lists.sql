-- +goose Up
ALTER TABLE lists ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;
CREATE INDEX lists_import ON lists (import_id) WHERE import_id IS NOT NULL;

-- +goose Down
DROP INDEX lists_import;
ALTER TABLE lists DROP COLUMN import_id;
