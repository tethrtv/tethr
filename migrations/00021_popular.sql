-- +goose Up
ALTER TABLE profiles ADD COLUMN in_popular boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE profiles DROP COLUMN in_popular;
