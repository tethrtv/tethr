-- +goose Up
ALTER TABLE accounts ADD COLUMN time_zone text;
ALTER TABLE accounts ADD COLUMN weekly_email boolean NOT NULL DEFAULT true;
ALTER TABLE accounts ADD COLUMN weekly_sent_at timestamptz;
ALTER TABLE accounts ADD COLUMN unsubscribe_token uuid NOT NULL UNIQUE DEFAULT gen_random_uuid();

-- +goose Down
ALTER TABLE accounts DROP COLUMN unsubscribe_token;
ALTER TABLE accounts DROP COLUMN weekly_sent_at;
ALTER TABLE accounts DROP COLUMN weekly_email;
ALTER TABLE accounts DROP COLUMN time_zone;
