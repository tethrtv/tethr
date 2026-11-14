-- +goose Up

ALTER TABLE wikidata_pending ADD COLUMN batch text;

CREATE TABLE review_entries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('kept_name', 'held_change', 'kind_change', 'shared_id', 'tmdb_differs', 'deleted_item', 'film_and_show', 'unreadable_class', 'unmapped_value')),
    key text NOT NULL,
    qid text NOT NULL,
    title_id bigint REFERENCES titles (id) ON DELETE CASCADE,
    subject text NOT NULL DEFAULT '',
    tethr_value text NOT NULL DEFAULT '',
    wikidata_value text NOT NULL DEFAULT '',
    why text NOT NULL DEFAULT '',
    other_id bigint REFERENCES titles (id) ON DELETE SET NULL,
    batch text,
    created_at timestamptz NOT NULL DEFAULT now(),
    cleared_at timestamptz,
    outcome text CHECK (outcome IN ('fixed', 'accepted', 'dismissed')),
    reason text
);

CREATE UNIQUE INDEX review_entries_open ON review_entries (key) WHERE cleared_at IS NULL;
CREATE INDEX review_entries_dismissed ON review_entries (key) WHERE outcome = 'dismissed';
CREATE INDEX review_entries_title ON review_entries (title_id) WHERE cleared_at IS NULL;
CREATE INDEX review_entries_batch ON review_entries (batch) WHERE cleared_at IS NULL AND batch IS NOT NULL;

-- +goose Down

DROP TABLE review_entries;
ALTER TABLE wikidata_pending DROP COLUMN batch;
