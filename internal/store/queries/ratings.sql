-- name: Ratings :many
SELECT r.*
FROM ratings r
JOIN titles t ON t.id = r.title_id
WHERE r.profile_id = $1
  AND (sqlc.narg(kind)::text IS NULL
    OR (sqlc.narg(kind) = 'movie' AND t.kind = 'movie')
    OR (sqlc.narg(kind) = 'show' AND t.kind = 'show' AND r.season_number IS NULL AND r.episode_id IS NULL)
    OR (sqlc.narg(kind) = 'season' AND r.season_number IS NOT NULL)
    OR (sqlc.narg(kind) = 'episode' AND r.episode_id IS NOT NULL))
  AND (cardinality(@ratings::smallint[]) = 0 OR r.rating = ANY(@ratings::smallint[]))
ORDER BY r.rated_at DESC, r.id DESC;

-- name: RateItem :one
INSERT INTO ratings (profile_id, title_id, season_number, episode_id, rating, rated_at)
VALUES ($1, $2, $3, $4, $5, coalesce(sqlc.narg(rated_at)::timestamptz, now()))
ON CONFLICT (profile_id, title_id, season_number, episode_id)
DO UPDATE SET rating = excluded.rating, rated_at = excluded.rated_at, import_id = NULL
RETURNING *;

-- name: UnrateItem :execrows
DELETE FROM ratings
WHERE profile_id = $1 AND title_id = $2
  AND season_number IS NOT DISTINCT FROM sqlc.narg(season_number)
  AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id);

-- name: DeleteRating :one
DELETE FROM ratings r WHERE r.id = $1 AND r.profile_id = $2
RETURNING r.season_number, r.episode_id, (SELECT kind FROM titles t WHERE t.id = r.title_id)::text AS kind;

-- name: WatchedMovies :many
SELECT title_id, count(*) AS plays, max(watched_at)::timestamptz AS last_watched_at
FROM watch_events
WHERE profile_id = $1 AND episode_id IS NULL
GROUP BY title_id
ORDER BY max(watched_at) DESC;

-- name: WatchedEpisodes :many
SELECT w.title_id, w.episode_id, e.season_number, e.episode_number,
    count(*) AS plays, max(w.watched_at)::timestamptz AS last_watched_at
FROM watch_events w
JOIN episodes e ON e.id = w.episode_id
WHERE w.profile_id = $1 AND (sqlc.narg(show_id)::bigint IS NULL OR w.title_id = sqlc.narg(show_id))
GROUP BY w.title_id, w.episode_id, e.season_number, e.episode_number
ORDER BY w.title_id, e.season_number, e.episode_number;

-- name: AiredEpisodes :many
SELECT * FROM episodes
WHERE show_id = $1 AND removed_at IS NULL AND first_aired <= current_date
  AND (sqlc.arg(specials)::boolean OR season_number > 0)
ORDER BY season_number, episode_number;

-- name: HiddenSeasons :many
SELECT season_number::integer FROM hidden_items
WHERE profile_id = $1 AND section = 'progress_watched' AND title_id = $2 AND season_number IS NOT NULL
ORDER BY season_number;
