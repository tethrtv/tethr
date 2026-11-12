-- +goose Up

CREATE TABLE wikidata_classes (
    qid text PRIMARY KEY,
    label text NOT NULL DEFAULT '',
    country text NOT NULL DEFAULT '',
    language text NOT NULL DEFAULT '',
    parents text[] NOT NULL DEFAULT '{}',
    checked_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE titles
    ADD COLUMN wikidata_instance_of text[] NOT NULL DEFAULT '{}',
    ADD COLUMN wikidata_genres text[] NOT NULL DEFAULT '{}',
    ADD COLUMN wikidata_countries text[] NOT NULL DEFAULT '{}',
    ADD COLUMN wikidata_languages text[] NOT NULL DEFAULT '{}',
    ADD COLUMN sitelinks integer NOT NULL DEFAULT 0,
    ADD COLUMN started text CHECK (started ~ '^[0-9]{4}(-[0-9]{2}(-[0-9]{2})?)?$'),
    ADD COLUMN ended text CHECK (ended ~ '^[0-9]{4}(-[0-9]{2}(-[0-9]{2})?)?$'),
    ADD COLUMN genres text[] NOT NULL DEFAULT '{}',
    ADD COLUMN type_flags text[] NOT NULL DEFAULT '{}',
    ADD COLUMN adult boolean NOT NULL DEFAULT false,
    ADD COLUMN countries text[] NOT NULL DEFAULT '{}',
    ADD COLUMN languages text[] NOT NULL DEFAULT '{}',
    ADD COLUMN facts_version text;

UPDATE titles SET wikidata_checked_at = NULL WHERE wikidata_id IS NOT NULL;

-- +goose Down

ALTER TABLE titles
    DROP COLUMN wikidata_instance_of,
    DROP COLUMN wikidata_genres,
    DROP COLUMN wikidata_countries,
    DROP COLUMN wikidata_languages,
    DROP COLUMN sitelinks,
    DROP COLUMN started,
    DROP COLUMN ended,
    DROP COLUMN genres,
    DROP COLUMN type_flags,
    DROP COLUMN adult,
    DROP COLUMN countries,
    DROP COLUMN languages,
    DROP COLUMN facts_version;

DROP TABLE wikidata_classes;
