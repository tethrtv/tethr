-- name: CreateList :one
INSERT INTO lists (profile_id, kind, name, slug, description, privacy, sort_by, sort_how)
VALUES ($1, 'custom', $2, $3, $4, $5, $6, $7)
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
SET name = $2, slug = $3, description = $4, privacy = $5, sort_by = $6, sort_how = $7, cover_item_id = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteList :exec
DELETE FROM lists WHERE id = $1;

-- name: VisibleLists :many
SELECT sqlc.embed(l), o.username AS owner_username, o.name AS owner_name, o.private AS owner_private,
    CASE WHEN l.profile_id = sqlc.arg(viewer)::bigint THEN 'mine'
         WHEN o.household_id = v.household_id THEN 'household'
         ELSE 'public' END::text AS access,
    (SELECT count(*) FROM list_items i WHERE i.list_id = l.id)::bigint AS item_count,
    coalesce((SELECT max(t.wikidata_checked_at) FROM list_items i JOIN titles t ON t.id = i.title_id WHERE i.list_id = l.id), 'epoch')::timestamptz AS titles_checked_at
FROM lists l
JOIN profiles o ON o.id = l.profile_id
LEFT JOIN profiles v ON v.id = sqlc.arg(viewer)::bigint
WHERE l.kind = 'custom'
  AND (sqlc.narg(list_id)::bigint IS NULL OR l.id = sqlc.narg(list_id))
  AND (sqlc.narg(owner)::bigint IS NULL OR l.profile_id = sqlc.narg(owner))
  AND (sqlc.narg(list_id)::bigint IS NOT NULL OR sqlc.narg(owner)::bigint IS NOT NULL OR o.household_id = v.household_id)
  AND (l.profile_id = sqlc.arg(viewer)::bigint OR l.privacy = 'public' OR (l.privacy = 'household' AND o.household_id = v.household_id))
ORDER BY l.profile_id <> sqlc.arg(viewer)::bigint, o.position, lower(l.name), l.id;

-- name: ProfileByUsername :one
SELECT * FROM profiles WHERE username = lower($1);

-- name: HouseholdOf :many
SELECT p.id FROM profiles p
JOIN profiles o ON o.household_id = p.household_id
WHERE o.id = $1;

-- name: ListItemsIn :many
SELECT li.*, t.kind AS title_kind
FROM list_items li
JOIN titles t ON t.id = li.title_id
WHERE li.list_id = ANY(sqlc.arg(list_ids)::bigint[])
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
SELECT id FROM list_items WHERE list_id = $1;

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
  AND i.title_id = sqlc.arg(title_id) AND i.season_number IS NULL AND i.episode_id IS NULL
ORDER BY i.list_id;
