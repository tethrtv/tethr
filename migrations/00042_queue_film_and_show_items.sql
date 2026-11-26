-- +goose Up

-- Twins from before the classes decided the kind only go once their item is fetched again.
INSERT INTO wikidata_pending (qid, edited_at, last_edited_at, not_before)
SELECT wikidata_id, now(), NULL::timestamptz, now() FROM titles
WHERE wikidata_id IS NOT NULL AND merged_into_id IS NULL AND wikidata_deleted_at IS NULL
GROUP BY wikidata_id
HAVING count(DISTINCT kind) > 1
ON CONFLICT (qid) DO NOTHING;

-- +goose Down
