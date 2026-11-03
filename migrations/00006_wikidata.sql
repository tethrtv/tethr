-- +goose Up
ALTER TABLE titles
    ADD COLUMN original_name text,
    ADD COLUMN runtime_minutes smallint,
    ADD COLUMN wikidata_id text,
    ADD COLUMN wikidata_checked_at timestamptz,
    ADD COLUMN merged_into_id bigint REFERENCES titles (id);

CREATE INDEX titles_wikidata ON titles (wikidata_id);

-- +goose Down
DROP INDEX titles_wikidata;
ALTER TABLE titles
    DROP COLUMN merged_into_id,
    DROP COLUMN wikidata_checked_at,
    DROP COLUMN wikidata_id,
    DROP COLUMN runtime_minutes,
    DROP COLUMN original_name;
