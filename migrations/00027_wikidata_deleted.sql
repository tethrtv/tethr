-- +goose Up

ALTER TABLE titles ADD COLUMN wikidata_deleted_at timestamptz;

-- +goose Down

ALTER TABLE titles DROP COLUMN wikidata_deleted_at;
