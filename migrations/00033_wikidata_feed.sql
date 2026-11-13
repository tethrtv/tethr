-- +goose Up

CREATE TABLE wikidata_feed (
    one boolean PRIMARY KEY DEFAULT true CHECK (one),
    cursor timestamptz NOT NULL
);

-- A seeded catalogue follows the feed from the moment its snapshot was taken.
INSERT INTO wikidata_feed (cursor)
SELECT CASE WHEN EXISTS (SELECT 1 FROM titles WHERE wikidata_id IS NOT NULL) THEN timestamptz '2026-11-10 16:50:45+00' ELSE now() END;

CREATE TABLE wikidata_pending (
    qid text PRIMARY KEY,
    edited_at timestamptz NOT NULL,
    last_edited_at timestamptz,
    not_before timestamptz NOT NULL,
    logged_out boolean NOT NULL DEFAULT false,
    held jsonb
);

CREATE INDEX wikidata_pending_not_before ON wikidata_pending (not_before);

-- +goose Down

DROP TABLE wikidata_pending;
DROP TABLE wikidata_feed;
