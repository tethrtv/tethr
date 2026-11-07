-- +goose Up
CREATE TABLE up_next_lists (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    list_id bigint NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, list_id)
);

CREATE INDEX up_next_lists_list ON up_next_lists (list_id);

-- +goose Down
DROP TABLE up_next_lists;
