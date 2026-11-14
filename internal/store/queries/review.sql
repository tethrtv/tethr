-- name: FlagReview :exec
INSERT INTO review_entries (kind, key, qid, title_id, subject, tethr_value, wikidata_value, why, other_id, batch)
SELECT f.kind, f.key, f.qid, nullif(f.title_id, 0), f.subject, f.tethr_value, f.wikidata_value, f.why, nullif(f.other_id, 0), nullif(f.batch, '')
FROM (SELECT unnest(@kinds::text[]) AS kind, unnest(@keys::text[]) AS key, unnest(@qids::text[]) AS qid, unnest(@title_ids::bigint[]) AS title_id,
        unnest(@subjects::text[]) AS subject, unnest(@tethr_values::text[]) AS tethr_value, unnest(@wikidata_values::text[]) AS wikidata_value,
        unnest(@whys::text[]) AS why, unnest(@other_ids::bigint[]) AS other_id, unnest(@batches::text[]) AS batch) f
WHERE NOT EXISTS (
    SELECT 1 FROM review_entries d
    WHERE d.key = f.key AND d.outcome = 'dismissed' AND d.tethr_value = f.tethr_value AND d.wikidata_value = f.wikidata_value
)
  AND NOT EXISTS (
    SELECT 1 FROM review_entries r
    WHERE r.key = f.key AND r.cleared_at IS NULL AND r.kind = 'held_change' AND r.tethr_value = f.wikidata_value AND r.wikidata_value = f.tethr_value
)
ON CONFLICT (key) WHERE cleared_at IS NULL DO UPDATE
SET tethr_value = excluded.tethr_value, wikidata_value = excluded.wikidata_value, why = excluded.why, other_id = excluded.other_id,
    batch = coalesce(review_entries.batch, excluded.batch);

-- name: SettleReview :exec
UPDATE review_entries SET cleared_at = now(), outcome = 'fixed'
WHERE cleared_at IS NULL AND title_id = ANY(@title_ids::bigint[]) AND kind = ANY(@kinds::text[]) AND kind <> 'held_change'
  AND NOT key = ANY(@keep::text[]);

-- name: SettleHeldChanges :exec
UPDATE review_entries e SET cleared_at = now(), outcome = 'fixed'
FROM titles t
WHERE e.cleared_at IS NULL AND e.kind = 'held_change' AND e.title_id = t.id AND t.id = ANY(@title_ids::bigint[])
  AND ((e.subject = 'name' AND t.name IS DISTINCT FROM e.wikidata_value) OR (e.subject = 'year' AND t.year::text IS DISTINCT FROM e.wikidata_value));

-- name: ReplaceReview :exec
UPDATE review_entries SET cleared_at = now(), outcome = 'fixed'
WHERE cleared_at IS NULL AND kind = @kind AND NOT key = ANY(@keep::text[]);

-- name: ClearReviewKeys :exec
UPDATE review_entries SET cleared_at = now(), outcome = 'fixed'
WHERE cleared_at IS NULL AND key = ANY(@keys::text[]);

-- name: OpenReview :many
SELECT e.id, e.kind, e.qid, e.title_id, e.subject, e.tethr_value, e.wikidata_value, e.why, e.other_id, e.batch, e.created_at,
    t.name AS title_name, t.year AS title_year, t.kind AS title_kind, t.slug AS title_slug, t.tmdb_id AS title_tmdb, coalesce(t.sitelinks, 0)::int AS sitelinks,
    o.name AS other_name, o.wikidata_id AS other_qid,
    (CASE WHEN e.kind = 'title_report' THEN count(*) OVER (PARTITION BY e.kind, e.title_id, e.subject) ELSE 0 END)::int AS reports
FROM review_entries e
LEFT JOIN titles t ON t.id = e.title_id
LEFT JOIN titles o ON o.id = e.other_id
WHERE e.cleared_at IS NULL
ORDER BY e.kind <> 'title_report', reports DESC, e.kind, coalesce(t.sitelinks, 0) DESC, e.title_id, e.subject, e.id;

-- name: ReviewTargets :many
SELECT e.id, e.kind, e.title_id, e.wikidata_value FROM review_entries e
WHERE e.cleared_at IS NULL AND (e.id = @id OR e.batch = @batch OR (e.kind = 'title_report' AND e.id <= @id AND (e.title_id, e.subject) IN (
    SELECT r.title_id, r.subject FROM review_entries r WHERE r.id = @id AND r.kind = 'title_report' AND r.cleared_at IS NULL)))
ORDER BY e.id
FOR UPDATE;

-- name: CloseReview :exec
UPDATE review_entries SET cleared_at = now(), outcome = @outcome, reason = sqlc.narg(reason)
WHERE id = ANY(@ids::bigint[]);

-- name: SetTitleName :exec
UPDATE titles SET name = $2, updated_at = now() WHERE id = $1;

-- name: PruneReview :exec
DELETE FROM review_entries WHERE cleared_at < now() - interval '1 year';

-- name: TitlesWikidata :many
SELECT id, wikidata_id FROM titles WHERE id = ANY(@ids::bigint[]);

-- name: FilmAndShowItems :many
SELECT wikidata_id::text AS qid,
    min(id) FILTER (WHERE kind = 'movie')::bigint AS film_id,
    min(id) FILTER (WHERE kind = 'show')::bigint AS show_id
FROM titles
WHERE merged_into_id IS NULL AND wikidata_id IS NOT NULL
GROUP BY wikidata_id
HAVING count(DISTINCT kind) > 1;

-- name: RawValueCounts :many
SELECT v.field, v.qid, v.titles, coalesce(c.label, '')::text AS label
FROM (
    SELECT 'genre'::text AS field, g::text AS qid, count(*) AS titles FROM titles, unnest(wikidata_genres) AS g
    WHERE merged_into_id IS NULL GROUP BY g HAVING count(*) >= @at_least::int
    UNION ALL
    SELECT 'country', c, count(*) FROM titles, unnest(wikidata_countries) AS c
    WHERE merged_into_id IS NULL GROUP BY c HAVING count(*) >= @at_least::int
    UNION ALL
    SELECT 'language', l, count(*) FROM titles, unnest(wikidata_languages) AS l
    WHERE merged_into_id IS NULL GROUP BY l HAVING count(*) >= @at_least::int
) v
LEFT JOIN wikidata_classes c ON c.qid = v.qid
ORDER BY v.titles DESC, v.qid;

-- name: OpenReviewSubjects :many
SELECT DISTINCT subject FROM review_entries
WHERE cleared_at IS NULL AND kind = @kind AND title_id = ANY(@title_ids::bigint[]);

-- name: SharedIDsLetGo :many
SELECT DISTINCT e.qid FROM review_entries e
JOIN titles o ON o.id = e.other_id
WHERE e.cleared_at IS NULL AND e.kind = 'shared_id' AND e.other_id = ANY(@title_ids::bigint[])
  AND e.subject <> 'tmdb ' || coalesce(o.tmdb_id::text, '')
  AND NOT EXISTS (SELECT 1 FROM external_ids x WHERE x.title_id = e.other_id AND x.provider || ' ' || x.value = e.subject);

-- name: RecheckWikidata :exec
UPDATE titles SET wikidata_checked_at = NULL WHERE id = $1;

-- name: LockHousehold :one
SELECT id FROM households WHERE id = $1 FOR UPDATE;

-- name: CountTitleReportsSince :one
SELECT count(*) FROM review_entries WHERE kind = 'title_report' AND household_id = $1 AND created_at > $2;

-- name: AddTitleReport :execrows
INSERT INTO review_entries (kind, key, qid, title_id, subject, tethr_value, wikidata_value, why, other_id, household_id, reporter_id)
VALUES ('title_report', @key, @qid, @title_id, @subject, @tethr_value, @wikidata_value, @why, sqlc.narg(other_id), @household_id, @reporter_id)
ON CONFLICT (key) WHERE cleared_at IS NULL DO UPDATE
SET wikidata_value = CASE WHEN excluded.wikidata_value <> '' THEN excluded.wikidata_value ELSE review_entries.wikidata_value END,
    why = CASE WHEN excluded.why <> '' THEN excluded.why ELSE review_entries.why END,
    other_id = coalesce(excluded.other_id, review_entries.other_id),
    reporter_id = excluded.reporter_id
WHERE (excluded.wikidata_value <> '' AND excluded.wikidata_value <> review_entries.wikidata_value)
   OR (excluded.why <> '' AND excluded.why <> review_entries.why)
   OR (excluded.other_id IS NOT NULL AND excluded.other_id IS DISTINCT FROM review_entries.other_id);

-- name: SettleTitleReports :exec
UPDATE review_entries e SET cleared_at = now(), outcome = 'fixed'
FROM titles t
WHERE e.cleared_at IS NULL AND e.kind = 'title_report' AND e.title_id = t.id AND t.id = ANY(@title_ids::bigint[]) AND e.wikidata_value <> ''
  AND ((e.subject = 'name' AND t.name = e.wikidata_value)
    OR (e.subject = 'year' AND t.year::text = e.wikidata_value)
    OR (e.subject = 'kind' AND EXISTS (SELECT 1 FROM titles n WHERE n.wikidata_id = t.wikidata_id AND n.kind = e.wikidata_value AND n.merged_into_id IS NULL))
    OR (e.subject = 'link tmdb' AND t.tmdb_id::text = e.wikidata_value)
    OR (e.subject LIKE 'link %' AND EXISTS (SELECT 1 FROM external_ids x WHERE x.title_id = t.id AND 'link ' || x.provider = e.subject AND x.value = e.wikidata_value)
        AND NOT EXISTS (SELECT 1 FROM external_ids y WHERE y.title_id = t.id AND 'link ' || y.provider = e.subject AND y.value <> e.wikidata_value))
    OR (e.subject = 'duplicate' AND e.other_id IS NOT NULL AND (t.merged_into_id = e.other_id
        OR EXISTS (SELECT 1 FROM titles o WHERE o.id = e.other_id AND o.merged_into_id = t.id))));
