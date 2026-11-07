-- +goose Up
ALTER TABLE invites ADD COLUMN created_by_account_id bigint REFERENCES accounts (id) ON DELETE CASCADE;
CREATE INDEX invites_created_by ON invites (created_by_account_id);
ALTER TABLE accounts ADD COLUMN extra_invites integer NOT NULL DEFAULT 0;

CREATE TABLE switches (
    name text PRIMARY KEY,
    set_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE switches;
ALTER TABLE accounts DROP COLUMN extra_invites;
ALTER TABLE invites DROP COLUMN created_by_account_id;
