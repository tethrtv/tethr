-- +goose Up
CREATE TABLE apps (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_account_id bigint REFERENCES accounts (id) ON DELETE CASCADE,
    client_id text NOT NULL UNIQUE,
    client_secret_hash bytea NOT NULL,
    name text NOT NULL,
    redirect_uris text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE oauth_grants (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_id bigint NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    any_profile boolean NOT NULL DEFAULT false,
    scopes text[] NOT NULL DEFAULT '{public}',
    label text,
    user_agent text,
    approved_via text NOT NULL CHECK (approved_via IN ('device', 'redirect')),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);

CREATE INDEX oauth_grants_profile ON oauth_grants (profile_id);
CREATE INDEX oauth_grants_app ON oauth_grants (app_id) WHERE revoked_at IS NULL;

CREATE TABLE oauth_tokens (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    grant_id bigint NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    access_token_hash bytea NOT NULL UNIQUE,
    refresh_token_hash bytea NOT NULL UNIQUE,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE INDEX oauth_tokens_grant ON oauth_tokens (grant_id);

CREATE TABLE device_codes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_id bigint NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    device_code_hash bytea NOT NULL UNIQUE,
    user_code text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'used')),
    profile_id bigint REFERENCES profiles (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    interval_seconds integer NOT NULL,
    last_polled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX device_codes_pending ON device_codes (user_code) WHERE status = 'pending';

CREATE TABLE oauth_codes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_id bigint NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    code_hash bytea NOT NULL UNIQUE,
    redirect_uri text NOT NULL,
    code_challenge text,
    code_challenge_method text,
    expires_at timestamptz NOT NULL,
    used_at timestamptz
);

-- Trakt clients cache Trakt's own IDs, so ours start well above anything Trakt has issued.
CREATE SEQUENCE catalogue_id START 1000000000 MAXVALUE 2147483647;

CREATE TABLE titles (
    id bigint PRIMARY KEY DEFAULT nextval('catalogue_id'),
    kind text NOT NULL CHECK (kind IN ('movie', 'show')),
    slug text NOT NULL UNIQUE,
    name text,
    year smallint,
    tmdb_id integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, tmdb_id)
);

CREATE TABLE external_ids (
    title_id bigint NOT NULL REFERENCES titles (id),
    provider text NOT NULL CHECK (provider IN ('imdb', 'tvdb')),
    value text NOT NULL,
    PRIMARY KEY (provider, value)
);

CREATE INDEX external_ids_title ON external_ids (title_id);

CREATE TABLE episodes (
    id bigint PRIMARY KEY DEFAULT nextval('catalogue_id'),
    show_id bigint NOT NULL REFERENCES titles (id),
    season_number integer NOT NULL,
    episode_number integer NOT NULL,
    tmdb_episode_id integer UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (show_id, season_number, episode_number)
);

CREATE TABLE watch_events (
    id bigint GENERATED ALWAYS AS IDENTITY (START WITH 1000000000000) PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    episode_id bigint REFERENCES episodes (id),
    watched_at timestamptz NOT NULL,
    action text NOT NULL CHECK (action IN ('scrobble', 'checkin', 'watch')),
    grant_id bigint REFERENCES oauth_grants (id) ON DELETE SET NULL,
    duration_seconds numeric(9, 3),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX watch_events_history ON watch_events (profile_id, watched_at DESC, id DESC);
CREATE INDEX watch_events_item ON watch_events (profile_id, title_id, episode_id);
CREATE INDEX watch_events_grant ON watch_events (grant_id);

CREATE TABLE playback (
    id bigint GENERATED ALWAYS AS IDENTITY (START WITH 1000000000000) PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    episode_id bigint REFERENCES episodes (id),
    progress numeric(5, 2) NOT NULL,
    paused_at timestamptz NOT NULL,
    grant_id bigint REFERENCES oauth_grants (id) ON DELETE SET NULL,
    duration_seconds numeric(9, 3),
    UNIQUE NULLS NOT DISTINCT (profile_id, title_id, episode_id)
);

CREATE INDEX playback_recent ON playback (profile_id, paused_at DESC);

CREATE TABLE watching (
    profile_id bigint PRIMARY KEY REFERENCES profiles (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    episode_id bigint REFERENCES episodes (id),
    grant_id bigint REFERENCES oauth_grants (id) ON DELETE SET NULL,
    progress numeric(5, 2) NOT NULL,
    started_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE activity_marks (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    area text NOT NULL,
    at timestamptz NOT NULL,
    PRIMARY KEY (profile_id, area)
);

CREATE TABLE lists (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('watchlist', 'custom')),
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    privacy text NOT NULL DEFAULT 'private' CHECK (privacy IN ('private', 'public')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (profile_id, slug)
);

CREATE UNIQUE INDEX lists_one_watchlist ON lists (profile_id) WHERE kind = 'watchlist';

CREATE TABLE list_items (
    id bigint GENERATED ALWAYS AS IDENTITY (START WITH 1000000000000) PRIMARY KEY,
    list_id bigint NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    season_number integer,
    episode_id bigint REFERENCES episodes (id),
    rank integer NOT NULL,
    notes text,
    listed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (list_id, title_id, season_number, episode_id)
);

CREATE INDEX list_items_rank ON list_items (list_id, rank);

-- +goose Down
DROP TABLE list_items;
DROP TABLE lists;
DROP TABLE activity_marks;
DROP TABLE watching;
DROP TABLE playback;
DROP TABLE watch_events;
DROP TABLE episodes;
DROP TABLE external_ids;
DROP TABLE titles;
DROP SEQUENCE catalogue_id;
DROP TABLE oauth_codes;
DROP TABLE device_codes;
DROP TABLE oauth_tokens;
DROP TABLE oauth_grants;
DROP TABLE apps;
