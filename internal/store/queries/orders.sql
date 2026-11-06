-- name: SaveEpisodeOrder :one
INSERT INTO episode_orders (show_id, tmdb_group_id, tmdb_type)
VALUES ($1, $2, $3)
ON CONFLICT (tmdb_group_id) DO UPDATE SET tmdb_type = excluded.tmdb_type, refreshed_at = now()
RETURNING id, show_id;

-- name: ClearEpisodeOrder :exec
DELETE FROM episode_order_entries WHERE order_id = $1;

-- name: AddEpisodeOrderEntries :copyfrom
INSERT INTO episode_order_entries (order_id, tmdb_episode_id, season_number, episode_number, position)
VALUES ($1, $2, $3, $4, $5);

-- name: SetShowOrder :exec
INSERT INTO profile_show_settings (profile_id, show_id, episode_order_id)
VALUES ($1, $2, $3)
ON CONFLICT (profile_id, show_id) DO UPDATE SET episode_order_id = excluded.episode_order_id, updated_at = now();

-- name: ShowOrder :one
SELECT o.tmdb_group_id
FROM profile_show_settings s
JOIN episode_orders o ON o.id = s.episode_order_id
WHERE s.profile_id = $1 AND s.show_id = $2;

-- name: OrderedEpisodes :many
SELECT s.show_id, sqlc.embed(e), x.season_number AS order_season, x.episode_number AS order_number
FROM profile_show_settings s
JOIN episode_order_entries x ON x.order_id = s.episode_order_id
JOIN episodes e ON e.tmdb_episode_id = x.tmdb_episode_id AND e.show_id = s.show_id AND e.removed_at IS NULL
WHERE s.profile_id = $1 AND s.show_id = ANY(sqlc.arg(show_ids)::bigint[])
ORDER BY s.show_id, x.position;

-- name: DueEpisodeOrders :many
SELECT tmdb_group_id FROM episode_orders o
WHERE refreshed_at < now() - interval '30 days'
  AND EXISTS (SELECT 1 FROM profile_show_settings s WHERE s.episode_order_id = o.id)
ORDER BY refreshed_at
LIMIT $1;

-- name: DropUnusedEpisodeOrders :exec
DELETE FROM episode_orders o
WHERE NOT EXISTS (SELECT 1 FROM profile_show_settings s WHERE s.episode_order_id = o.id);

-- name: DropTitleShowSettings :exec
DELETE FROM profile_show_settings WHERE show_id = $1;

-- name: DropTitleEpisodeOrders :exec
DELETE FROM episode_orders WHERE show_id = $1;

-- name: EpisodeOrderShow :one
SELECT show_id FROM episode_orders WHERE tmdb_group_id = $1;

-- name: ForgetEpisodeOrder :exec
UPDATE profile_show_settings SET episode_order_id = NULL, updated_at = now()
WHERE episode_order_id = (SELECT id FROM episode_orders WHERE tmdb_group_id = $1);
