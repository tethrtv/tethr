-- +goose Up
CREATE TABLE imports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN ('trakt')),
    filename text NOT NULL DEFAULT '',
    payload bytea,
    status text NOT NULL CHECK (status IN ('parsed', 'queued', 'applied', 'failed', 'undone')),
    summary jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    undone_at timestamptz
);

CREATE INDEX imports_profile ON imports (profile_id, created_at DESC);
CREATE INDEX imports_queued ON imports (created_at) WHERE status = 'queued';

CREATE TABLE ratings (
    id bigint GENERATED ALWAYS AS IDENTITY (START WITH 1000000000000) PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    season_number integer,
    episode_id bigint REFERENCES episodes (id),
    rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 10),
    rated_at timestamptz NOT NULL DEFAULT now(),
    import_id bigint REFERENCES imports (id) ON DELETE SET NULL,
    UNIQUE NULLS NOT DISTINCT (profile_id, title_id, season_number, episode_id)
);

ALTER TABLE watch_events ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;
ALTER TABLE playback ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;
ALTER TABLE list_items ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;
ALTER TABLE hidden_items ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;

CREATE INDEX watch_events_import ON watch_events (import_id) WHERE import_id IS NOT NULL;
CREATE INDEX playback_import ON playback (import_id) WHERE import_id IS NOT NULL;
CREATE INDEX list_items_import ON list_items (import_id) WHERE import_id IS NOT NULL;
CREATE INDEX hidden_items_import ON hidden_items (import_id) WHERE import_id IS NOT NULL;
CREATE INDEX ratings_import ON ratings (import_id) WHERE import_id IS NOT NULL;

-- +goose Down
ALTER TABLE hidden_items DROP COLUMN import_id;
ALTER TABLE list_items DROP COLUMN import_id;
ALTER TABLE playback DROP COLUMN import_id;
ALTER TABLE watch_events DROP COLUMN import_id;
DROP TABLE ratings;
DROP TABLE imports;
