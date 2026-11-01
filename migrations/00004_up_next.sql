-- +goose Up
ALTER TABLE titles
    ADD COLUMN outline_refreshed_at timestamptz,
    ADD COLUMN tmdb_missing_at timestamptz;

ALTER TABLE episodes
    ADD COLUMN first_aired date,
    ADD COLUMN removed_at timestamptz,
    DROP CONSTRAINT episodes_show_id_season_number_episode_number_key;

CREATE UNIQUE INDEX episodes_number ON episodes (show_id, season_number, episode_number) WHERE removed_at IS NULL;

CREATE TABLE hidden_items (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    section text NOT NULL CHECK (section IN ('calendar', 'progress_watched', 'progress_watched_reset', 'progress_collected', 'recommendations', 'comments', 'dropped')),
    title_id bigint NOT NULL REFERENCES titles (id),
    season_number integer,
    hidden_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (profile_id, section, title_id, season_number)
);

CREATE INDEX hidden_items_section ON hidden_items (profile_id, section);

-- +goose Down
DROP TABLE hidden_items;
DROP INDEX episodes_number;
ALTER TABLE episodes
    ADD CONSTRAINT episodes_show_id_season_number_episode_number_key UNIQUE (show_id, season_number, episode_number),
    DROP COLUMN removed_at,
    DROP COLUMN first_aired;
ALTER TABLE titles
    DROP COLUMN tmdb_missing_at,
    DROP COLUMN outline_refreshed_at;
