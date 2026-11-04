-- +goose Up
CREATE TABLE audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    profile_id bigint REFERENCES profiles (id) ON DELETE SET NULL,
    kind text NOT NULL,
    subject text,
    details jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_account ON audit_log (account_id, kind, created_at);
CREATE INDEX audit_log_created ON audit_log (created_at);

-- +goose Down
DROP TABLE audit_log;
