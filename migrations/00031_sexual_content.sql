-- +goose Up

ALTER TABLE titles ADD COLUMN sexual_content boolean NOT NULL DEFAULT false;
UPDATE titles SET sexual_content = adult;

-- +goose Down

ALTER TABLE titles DROP COLUMN sexual_content;
