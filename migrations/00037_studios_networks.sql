-- +goose Up

ALTER TABLE titles
    ADD COLUMN studios text[] NOT NULL DEFAULT '{}',
    ADD COLUMN networks text[] NOT NULL DEFAULT '{}';
ALTER TABLE wikidata_classes ADD COLUMN human boolean NOT NULL DEFAULT false;

-- Best-known titles first, so the pages people open fill in soonest.
INSERT INTO wikidata_pending (qid, edited_at, last_edited_at, not_before)
SELECT wikidata_id, now(), NULL::timestamptz, now() - make_interval(secs => max(sitelinks)) FROM titles
WHERE wikidata_id IS NOT NULL AND merged_into_id IS NULL AND wikidata_deleted_at IS NULL
GROUP BY wikidata_id
ON CONFLICT (qid) DO NOTHING;

CREATE INDEX wikidata_pending_due ON wikidata_pending ((last_edited_at IS NULL), not_before, qid);

-- +goose Down

DROP INDEX wikidata_pending_due;
ALTER TABLE wikidata_classes DROP COLUMN human;
ALTER TABLE titles DROP COLUMN networks, DROP COLUMN studios;
