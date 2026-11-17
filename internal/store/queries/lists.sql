-- name: CreateList :one
INSERT INTO lists (profile_id, kind, name, slug, description, privacy, sort_by, sort_how, rule)
VALUES ($1, 'custom', $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CountLists :one
SELECT count(*) FROM lists WHERE profile_id = $1 AND kind = 'custom';

-- name: ListSlugs :many
SELECT slug FROM lists
WHERE profile_id = $1 AND id <> sqlc.arg(except_id) AND (slug = sqlc.arg(slug)::text OR slug LIKE sqlc.arg(slug)::text || '-%');

-- name: OwnedList :one
SELECT * FROM lists WHERE id = $1 AND profile_id = $2 AND kind = 'custom' FOR UPDATE;

-- name: LockList :one
SELECT * FROM lists WHERE id = $1 AND kind = 'custom' FOR UPDATE;

-- name: UpdateList :one
UPDATE lists
SET name = $2, slug = $3, description = $4, privacy = $5, sort_by = $6, sort_how = $7, cover_item_id = $8, rule = $9, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteList :exec
DELETE FROM lists WHERE id = $1;

-- name: VisibleLists :many
SELECT sqlc.embed(l), o.username AS owner_username, o.name AS owner_name, o.private AS owner_private,
    CASE WHEN l.profile_id = sqlc.arg(viewer)::bigint THEN 'mine'
         WHEN o.household_id = v.household_id THEN 'household'
         ELSE 'public' END::text AS access,
    (SELECT count(*) FROM list_items i JOIN titles t ON t.id = i.title_id
     WHERE i.list_id = l.id AND NOT (sqlc.arg(for_kids)::bool AND (t.adult OR t.sexual_content)))::bigint AS item_count,
    coalesce((SELECT max(t.wikidata_checked_at) FROM list_items i JOIN titles t ON t.id = i.title_id WHERE i.list_id = l.id), 'epoch')::timestamptz AS titles_checked_at,
    EXISTS (SELECT 1 FROM up_next_lists u WHERE u.list_id = l.id AND u.profile_id = sqlc.arg(viewer)::bigint) AS in_up_next,
    (SELECT count(*) FROM list_follows f WHERE f.list_id = l.id)::bigint AS follower_count,
    EXISTS (SELECT 1 FROM list_follows f WHERE f.list_id = l.id AND f.profile_id = sqlc.arg(viewer)::bigint) AS following
FROM lists l
JOIN profiles o ON o.id = l.profile_id
LEFT JOIN profiles v ON v.id = sqlc.arg(viewer)::bigint
WHERE l.kind = 'custom'
  AND (sqlc.narg(list_id)::bigint IS NULL OR l.id = sqlc.narg(list_id))
  AND (sqlc.narg(owner)::bigint IS NULL OR l.profile_id = sqlc.narg(owner))
  AND (sqlc.narg(list_ids)::bigint[] IS NULL OR l.id = ANY(sqlc.narg(list_ids)::bigint[]))
  AND (sqlc.narg(list_id)::bigint IS NOT NULL OR sqlc.narg(owner)::bigint IS NOT NULL OR sqlc.narg(list_ids)::bigint[] IS NOT NULL OR o.household_id = v.household_id)
  AND (l.profile_id = sqlc.arg(viewer)::bigint OR l.privacy = 'public' OR (l.privacy = 'household' AND o.household_id = v.household_id))
ORDER BY l.profile_id <> sqlc.arg(viewer)::bigint, o.position, lower(l.name), l.id;

-- name: ProfileByUsername :one
SELECT * FROM profiles WHERE username = lower($1);

-- name: HouseholdOf :many
SELECT p.id FROM profiles p
JOIN profiles o ON o.household_id = p.household_id
WHERE o.id = $1;

-- name: ListItemsIn :many
SELECT li.*, t.kind AS title_kind, s.season_number
FROM list_items li
JOIN titles t ON t.id = li.title_id
LEFT JOIN seasons s ON s.id = li.season_id
WHERE li.list_id = ANY(sqlc.arg(list_ids)::bigint[])
  AND NOT (sqlc.arg(for_kids)::bool AND (t.adult OR t.sexual_content))
ORDER BY li.list_id, li.rank, li.id;

-- name: WatchedInTitles :many
SELECT title_id, episode_id, max(watched_at)::timestamptz AS last_watched_at
FROM watch_events
WHERE profile_id = $1 AND title_id = ANY(sqlc.arg(title_ids)::bigint[])
GROUP BY title_id, episode_id;

-- name: LatestActivity :one
SELECT coalesce(max(at), 'epoch')::timestamptz FROM activity_marks WHERE profile_id = $1;

-- name: SetListItemNotes :execrows
UPDATE list_items SET notes = $3 WHERE id = $1 AND list_id = $2;

-- name: RemoveListEntry :execrows
DELETE FROM list_items WHERE id = $1 AND list_id = $2;

-- name: RemoveListEntries :execrows
DELETE FROM list_items WHERE list_id = $1 AND id = ANY(sqlc.arg(ids)::bigint[]);

-- name: OnList :one
SELECT EXISTS (SELECT 1 FROM list_items WHERE id = $1 AND list_id = $2);

-- name: ListEntryIDs :many
SELECT li.id, coalesce(t.adult OR t.sexual_content, false)::bool AS flagged
FROM list_items li JOIN titles t ON t.id = li.title_id
WHERE li.list_id = $1
ORDER BY li.rank, li.id;

-- name: RankListItems :exec
UPDATE list_items li SET rank = x.rank::int
FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS x(id, rank)
WHERE li.id = x.id AND li.list_id = sqlc.arg(list_id);

-- name: TitleLists :many
SELECT i.list_id, i.id
FROM list_items i
JOIN lists l ON l.id = i.list_id
JOIN profiles o ON o.id = l.profile_id
JOIN profiles v ON v.id = sqlc.arg(profile_id)
WHERE l.kind = 'custom' AND o.household_id = v.household_id AND (l.profile_id = v.id OR l.privacy <> 'private')
  AND i.title_id = sqlc.arg(title_id) AND i.season_id IS NULL AND i.episode_id IS NULL
ORDER BY i.list_id;

-- name: TitleRuleLists :many
SELECT l.id, l.rule
FROM lists l
JOIN profiles o ON o.id = l.profile_id
JOIN profiles v ON v.id = sqlc.arg(profile_id)
WHERE l.kind = 'custom' AND l.rule IS NOT NULL AND o.household_id = v.household_id AND (l.profile_id = v.id OR l.privacy <> 'private')
ORDER BY l.id;

-- name: ListItemsAddedSince :many
SELECT list_id, title_id FROM list_items
WHERE list_id = ANY(sqlc.arg(list_ids)::bigint[]) AND listed_at >= sqlc.arg(since)
ORDER BY list_id, listed_at DESC, id DESC;

-- name: RuleTitles :many
SELECT t.id, t.kind, t.created_at, count(*) OVER ()::bigint AS matched
FROM titles t
WHERE t.merged_into_id IS NULL
  AND (sqlc.narg(kind)::text IS NULL OR t.kind = sqlc.narg(kind)::text)
  AND (cardinality(sqlc.arg(genres)::text[]) = 0 OR t.genres && sqlc.arg(genres)::text[])
  AND NOT t.genres && sqlc.arg(not_genres)::text[]
  AND (cardinality(sqlc.arg(types)::text[]) = 0 OR t.type_flags && sqlc.arg(types)::text[])
  AND NOT t.type_flags && sqlc.arg(not_types)::text[]
  AND (cardinality(sqlc.arg(studios)::text[]) = 0 OR t.studios && sqlc.arg(studios)::text[])
  AND NOT t.studios && sqlc.arg(not_studios)::text[]
  AND (cardinality(sqlc.arg(networks)::text[]) = 0 OR t.networks && sqlc.arg(networks)::text[])
  AND NOT t.networks && sqlc.arg(not_networks)::text[]
  AND (cardinality(sqlc.arg(countries)::text[]) = 0 OR t.countries && sqlc.arg(countries)::text[])
  AND NOT t.countries && sqlc.arg(not_countries)::text[]
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR t.languages && sqlc.arg(languages)::text[])
  AND NOT t.languages && sqlc.arg(not_languages)::text[]
  AND (sqlc.narg(year_from)::int IS NULL OR t.year >= sqlc.narg(year_from)::int)
  AND (sqlc.narg(year_to)::int IS NULL OR t.year <= sqlc.narg(year_to)::int)
  AND (sqlc.narg(runtime_from)::int IS NULL OR t.runtime_minutes >= sqlc.narg(runtime_from)::int)
  AND (sqlc.narg(runtime_to)::int IS NULL OR t.runtime_minutes <= sqlc.narg(runtime_to)::int)
  AND (sqlc.narg(watched)::bool IS NULL OR sqlc.narg(watched)::bool =
    (t.id IN (SELECT w.title_id FROM watch_events w WHERE w.profile_id = sqlc.arg(viewer)::bigint)))
  AND (sqlc.narg(rated)::bool IS NULL OR sqlc.narg(rated)::bool =
    (t.id IN (SELECT r.title_id FROM ratings r WHERE r.profile_id = sqlc.arg(viewer)::bigint AND r.season_id IS NULL AND r.episode_id IS NULL)))
  AND (sqlc.narg(watchlist)::bool IS NULL OR sqlc.narg(watchlist)::bool =
    (t.id IN (SELECT i.title_id FROM list_items i JOIN lists l ON l.id = i.list_id
              WHERE l.profile_id = sqlc.arg(viewer)::bigint AND l.kind = 'watchlist' AND i.season_id IS NULL AND i.episode_id IS NULL)))
  AND NOT t.id = ANY(sqlc.arg(left_out)::bigint[])
  AND t.id NOT IN (SELECT m.merged_into_id FROM titles m WHERE m.id = ANY(sqlc.arg(left_out)::bigint[]) AND m.merged_into_id IS NOT NULL)
  AND NOT (sqlc.arg(for_kids)::bool AND (t.adult OR t.sexual_content))
ORDER BY t.sitelinks DESC NULLS LAST, t.id
LIMIT 1000;

-- name: SearchCompanies :many
SELECT qid, label, studio_of, network_of FROM (
  SELECT c.qid, c.label,
      (SELECT count(*) FROM titles t WHERE t.merged_into_id IS NULL AND t.studios @> ARRAY[c.qid])::bigint AS studio_of,
      (SELECT count(*) FROM titles t WHERE t.merged_into_id IS NULL AND t.networks @> ARRAY[c.qid])::bigint AS network_of
  FROM wikidata_classes c
  WHERE NOT c.human AND c.label <> ''
    AND (c.label ILIKE sqlc.arg(search)::text || '%' OR c.label ILIKE '% ' || sqlc.arg(search)::text || '%')
) m
WHERE studio_of + network_of > 0
ORDER BY lower(label) = lower(sqlc.arg(search)::text) DESC, studio_of + network_of DESC, label
LIMIT 20;

-- name: SetListRule :exec
UPDATE lists SET rule = $2 WHERE id = $1;
