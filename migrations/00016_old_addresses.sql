-- +goose Up
CREATE TABLE old_usernames (
    username text PRIMARY KEY,
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX old_usernames_profile ON old_usernames (profile_id);

CREATE TABLE old_list_slugs (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    slug text NOT NULL,
    list_id bigint NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    changed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, slug)
);

CREATE INDEX old_list_slugs_list ON old_list_slugs (list_id);

-- +goose Down
DROP TABLE old_list_slugs;
DROP TABLE old_usernames;
