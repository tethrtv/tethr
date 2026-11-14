-- +goose Up

ALTER TABLE review_entries DROP CONSTRAINT review_entries_kind_check;
ALTER TABLE review_entries ADD CONSTRAINT review_entries_kind_check CHECK (kind IN ('kept_name', 'held_change', 'kind_change', 'shared_id', 'tmdb_differs', 'deleted_item', 'film_and_show', 'unreadable_class', 'unmapped_value', 'title_report'));
ALTER TABLE review_entries ADD COLUMN household_id bigint REFERENCES households (id) ON DELETE SET NULL;
ALTER TABLE review_entries ADD COLUMN reporter_id bigint REFERENCES profiles (id) ON DELETE SET NULL;

CREATE INDEX review_entries_household ON review_entries (household_id, created_at) WHERE kind = 'title_report';

-- +goose Down

DELETE FROM review_entries WHERE kind = 'title_report';
DROP INDEX review_entries_household;
ALTER TABLE review_entries DROP COLUMN reporter_id;
ALTER TABLE review_entries DROP COLUMN household_id;
ALTER TABLE review_entries DROP CONSTRAINT review_entries_kind_check;
ALTER TABLE review_entries ADD CONSTRAINT review_entries_kind_check CHECK (kind IN ('kept_name', 'held_change', 'kind_change', 'shared_id', 'tmdb_differs', 'deleted_item', 'film_and_show', 'unreadable_class', 'unmapped_value'));
