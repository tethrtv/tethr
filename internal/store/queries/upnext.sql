-- name: AddUpNextList :exec
INSERT INTO up_next_lists (profile_id, list_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveUpNextList :execrows
DELETE FROM up_next_lists WHERE profile_id = $1 AND list_id = $2;

-- name: CountUpNextLists :one
SELECT count(*) FROM up_next_lists WHERE profile_id = $1;

-- name: UpNextListIDs :many
SELECT list_id, added_at FROM up_next_lists WHERE profile_id = $1;

-- name: ItemPlayback :many
SELECT id, title_id, episode_id, progress::float8 AS progress FROM playback
WHERE profile_id = $1 AND title_id = ANY(sqlc.arg(title_ids)::bigint[]);
