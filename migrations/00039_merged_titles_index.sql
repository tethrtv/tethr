-- +goose Up
CREATE INDEX titles_merged_into ON titles (merged_into_id) WHERE merged_into_id IS NOT NULL;

-- +goose Down
DROP INDEX titles_merged_into;
