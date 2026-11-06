-- +goose Up
ALTER TABLE lists DROP CONSTRAINT lists_privacy_check;
ALTER TABLE lists ADD CONSTRAINT lists_privacy_check CHECK (privacy IN ('private', 'household', 'public'));
ALTER TABLE lists
    ADD COLUMN sort_by text NOT NULL DEFAULT 'rank' CHECK (sort_by IN ('rank', 'added', 'title', 'year')),
    ADD COLUMN sort_how text NOT NULL DEFAULT 'asc' CHECK (sort_how IN ('asc', 'desc')),
    ADD COLUMN import_id bigint REFERENCES imports (id) ON DELETE SET NULL;
-- Trakt's own list IDs are below a billion, so one a client cached never lands on ours.
ALTER TABLE lists ALTER COLUMN id RESTART WITH 1000000000;

-- +goose Down
UPDATE lists SET privacy = 'private' WHERE privacy = 'household';
ALTER TABLE lists DROP CONSTRAINT lists_privacy_check;
ALTER TABLE lists ADD CONSTRAINT lists_privacy_check CHECK (privacy IN ('private', 'public'));
ALTER TABLE lists DROP COLUMN sort_by, DROP COLUMN sort_how, DROP COLUMN import_id;
