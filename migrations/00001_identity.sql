-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email citext NOT NULL UNIQUE,
    email_verified_at timestamptz,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE households (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_account_id bigint NOT NULL UNIQUE REFERENCES accounts (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    household_id bigint NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    account_id bigint REFERENCES accounts (id) ON DELETE SET NULL,
    uuid uuid NOT NULL UNIQUE DEFAULT uuidv7(),
    username text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('owner', 'person', 'kids', 'guest')),
    hue smallint NOT NULL DEFAULT 95,
    private boolean NOT NULL DEFAULT true,
    paused boolean NOT NULL DEFAULT false,
    position integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX profiles_username_key ON profiles (lower(username));
CREATE UNIQUE INDEX profiles_one_owner ON profiles (household_id) WHERE kind = 'owner';

-- +goose Down
DROP TABLE profiles;
DROP TABLE households;
DROP TABLE accounts;
