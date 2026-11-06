-- +goose Up
ALTER TABLE apps ADD COLUMN official boolean NOT NULL DEFAULT false;

INSERT INTO apps (client_id, client_secret_hash, name, official)
VALUES ('tethr-jellyfin', '\x', 'Tethr for Jellyfin', true);

-- +goose Down
DELETE FROM apps WHERE official;
ALTER TABLE apps DROP COLUMN official;
