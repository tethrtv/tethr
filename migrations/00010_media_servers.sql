-- +goose Up
ALTER TABLE apps ADD COLUMN built_in boolean NOT NULL DEFAULT false;

INSERT INTO apps (client_id, client_secret_hash, name, built_in)
VALUES ('jellyfin', '\x', 'Jellyfin', true), ('plex', '\x', 'Plex', true), ('emby', '\x', 'Emby', true);

ALTER TABLE oauth_grants
    DROP CONSTRAINT oauth_grants_approved_via_check,
    ADD CONSTRAINT oauth_grants_approved_via_check CHECK (approved_via IN ('device', 'redirect', 'server'));

CREATE TABLE media_servers (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('jellyfin', 'plex', 'emby')),
    secret_hash bytea NOT NULL UNIQUE,
    name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_event_at timestamptz,
    revoked_at timestamptz
);

CREATE INDEX media_servers_account ON media_servers (account_id);

CREATE TABLE media_server_users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    server_id bigint NOT NULL REFERENCES media_servers (id) ON DELETE CASCADE,
    external_id text NOT NULL,
    name text NOT NULL,
    grant_id bigint REFERENCES oauth_grants (id) ON DELETE SET NULL,
    ignored boolean NOT NULL DEFAULT false,
    last_event_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (server_id, external_id)
);

CREATE TABLE held_plays (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    server_user_id bigint NOT NULL REFERENCES media_server_users (id) ON DELETE CASCADE,
    title_id bigint NOT NULL REFERENCES titles (id),
    episode_id bigint REFERENCES episodes (id),
    action text NOT NULL CHECK (action IN ('scrobble', 'watch')),
    watched_at timestamptz NOT NULL,
    duration_seconds numeric(9, 3)
);

CREATE INDEX held_plays_user ON held_plays (server_user_id, watched_at);

-- +goose Down
DROP TABLE held_plays;
DROP TABLE media_server_users;
DROP TABLE media_servers;
DELETE FROM oauth_grants WHERE approved_via = 'server';
ALTER TABLE oauth_grants
    DROP CONSTRAINT oauth_grants_approved_via_check,
    ADD CONSTRAINT oauth_grants_approved_via_check CHECK (approved_via IN ('device', 'redirect'));
DELETE FROM apps WHERE built_in;
ALTER TABLE apps DROP COLUMN built_in;
