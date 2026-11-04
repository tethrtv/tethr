-- +goose Up
UPDATE titles SET wikidata_checked_at = NULL WHERE name IS NOT NULL AND slug = 't-' || id;

-- +goose Down
