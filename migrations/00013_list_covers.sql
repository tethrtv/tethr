-- +goose Up
ALTER TABLE lists ADD COLUMN cover_item_id bigint REFERENCES list_items (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE lists DROP COLUMN cover_item_id;
