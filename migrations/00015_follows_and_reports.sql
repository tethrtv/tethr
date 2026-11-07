-- +goose Up
CREATE TABLE list_follows (
    profile_id bigint NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    list_id bigint NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    followed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, list_id)
);

CREATE INDEX list_follows_list ON list_follows (list_id);

CREATE TABLE list_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    list_id bigint NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    reporter_id bigint REFERENCES profiles (id) ON DELETE SET NULL,
    reason text NOT NULL CHECK (reason IN ('illegal', 'harassment', 'personal', 'spam', 'other')),
    note text,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    outcome text CHECK (outcome IN ('kept', 'removed'))
);

CREATE INDEX list_reports_open ON list_reports (list_id) WHERE resolved_at IS NULL;
CREATE UNIQUE INDEX list_reports_once ON list_reports (list_id, reporter_id) WHERE resolved_at IS NULL;
CREATE INDEX list_reports_reporter ON list_reports (reporter_id, created_at);

-- +goose Down
DROP TABLE list_reports;
DROP TABLE list_follows;
