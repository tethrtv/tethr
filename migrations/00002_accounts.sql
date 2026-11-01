-- +goose Up
CREATE TABLE email_links (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email citext NOT NULL,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    purpose text NOT NULL CHECK (purpose IN ('sign_in', 'sign_up')),
    token_hash bytea NOT NULL UNIQUE,
    code_hash bytea NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_links_email ON email_links (email, created_at DESC);

CREATE TABLE sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    profile_id bigint REFERENCES profiles (id) ON DELETE SET NULL,
    token_hash bytea NOT NULL UNIQUE,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE INDEX sessions_account ON sessions (account_id);

CREATE TABLE invites (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code text NOT NULL UNIQUE,
    note text,
    used_by_account_id bigint REFERENCES accounts (id) ON DELETE SET NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE passkeys (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    credential_id bytea NOT NULL UNIQUE,
    credential jsonb NOT NULL,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE INDEX passkeys_account ON passkeys (account_id);

CREATE TABLE auth_challenges (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE,
    account_id bigint REFERENCES accounts (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('register', 'sign_in')),
    session_data jsonb NOT NULL,
    expires_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE auth_challenges;
DROP TABLE passkeys;
DROP TABLE invites;
DROP TABLE sessions;
DROP TABLE email_links;
