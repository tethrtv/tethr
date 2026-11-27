-- +goose Up
ALTER TABLE titles ADD COLUMN released date;

INSERT INTO wikidata_pending (qid, edited_at, last_edited_at, not_before)
SELECT DISTINCT wikidata_id, now(), NULL::timestamptz, now() FROM titles
WHERE kind = 'movie' AND wikidata_id IS NOT NULL AND merged_into_id IS NULL AND wikidata_deleted_at IS NULL
ON CONFLICT (qid) DO NOTHING;

-- +goose Down
ALTER TABLE titles DROP COLUMN released;
