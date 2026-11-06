-- +goose Up
CREATE TABLE episode_orders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    show_id bigint NOT NULL REFERENCES titles (id),
    tmdb_group_id text NOT NULL UNIQUE,
    tmdb_type smallint NOT NULL,
    refreshed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX episode_orders_show ON episode_orders (show_id);

CREATE TABLE episode_order_entries (
    order_id bigint NOT NULL REFERENCES episode_orders (id) ON DELETE CASCADE,
    tmdb_episode_id integer NOT NULL,
    season_number integer NOT NULL,
    episode_number integer NOT NULL,
    position integer NOT NULL,
    PRIMARY KEY (order_id, tmdb_episode_id)
);

CREATE TABLE profile_show_settings (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    show_id bigint NOT NULL REFERENCES titles (id),
    episode_order_id bigint REFERENCES episode_orders (id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, show_id)
);

CREATE INDEX profile_show_settings_order ON profile_show_settings (episode_order_id);

-- +goose Down
DROP TABLE profile_show_settings;
DROP TABLE episode_order_entries;
DROP TABLE episode_orders;
