-- +goose Up
ALTER TABLE list_reports ADD COLUMN household_id bigint REFERENCES households (id) ON DELETE SET NULL;
UPDATE list_reports r SET household_id = p.household_id FROM profiles p WHERE p.id = r.reporter_id;
DELETE FROM list_reports a USING list_reports b
WHERE a.resolved_at IS NULL AND b.resolved_at IS NULL
  AND a.list_id = b.list_id AND a.household_id = b.household_id
  AND (b.reason IN ('illegal', 'personal'), -b.id) > (a.reason IN ('illegal', 'personal'), -a.id);

DROP INDEX list_reports_once;
DROP INDEX list_reports_reporter;
CREATE UNIQUE INDEX list_reports_once ON list_reports (list_id, household_id) WHERE resolved_at IS NULL;
CREATE INDEX list_reports_household ON list_reports (household_id, created_at);
CREATE INDEX list_reports_kept ON list_reports (household_id) WHERE outcome = 'kept';

ALTER TABLE lists ADD COLUMN kept_at timestamptz;
UPDATE lists l SET kept_at = k.at
FROM (SELECT list_id, max(resolved_at) AS at FROM list_reports WHERE outcome = 'kept' GROUP BY list_id) k
WHERE k.list_id = l.id;

CREATE VIEW hidden_lists AS
SELECT r.list_id
FROM list_reports r
JOIN lists l ON l.id = r.list_id
WHERE r.resolved_at IS NULL
  AND (l.kept_at IS NULL OR l.updated_at > l.kept_at)
  AND (SELECT count(*) FROM list_reports k WHERE k.household_id = r.household_id AND k.outcome = 'kept') < 3
GROUP BY r.list_id
HAVING bool_or(r.reason IN ('illegal', 'personal')) OR count(DISTINCT r.household_id) >= 3;

-- +goose Down
DROP VIEW hidden_lists;
ALTER TABLE lists DROP COLUMN kept_at;
DROP INDEX list_reports_kept;
DROP INDEX list_reports_household;
DROP INDEX list_reports_once;
CREATE UNIQUE INDEX list_reports_once ON list_reports (list_id, reporter_id) WHERE resolved_at IS NULL;
CREATE INDEX list_reports_reporter ON list_reports (reporter_id, created_at);
ALTER TABLE list_reports DROP COLUMN household_id;
