-- +goose Up

CREATE TABLE wikidata_left_out (
    qid text PRIMARY KEY,
    reason text NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE job_runs (
    name text PRIMARY KEY,
    ran_at timestamptz NOT NULL
);

-- +goose Down

DROP TABLE job_runs;
DROP TABLE wikidata_left_out;
