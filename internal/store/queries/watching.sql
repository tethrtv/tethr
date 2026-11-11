-- name: CreateWatchEvent :one
INSERT INTO watch_events (profile_id, title_id, episode_id, watched_at, action, grant_id, duration_seconds)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: RecentWatchEvent :one
SELECT * FROM watch_events
WHERE profile_id = $1 AND title_id = $2 AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id)
  AND action = 'scrobble' AND created_at > $3
ORDER BY created_at DESC
LIMIT 1;

-- name: History :many
SELECT w.*
FROM watch_events w
JOIN titles t ON t.id = w.title_id
WHERE w.profile_id = $1
  AND (sqlc.narg(kind)::text IS NULL
    OR (sqlc.narg(kind) = 'movie' AND t.kind = 'movie')
    OR (sqlc.narg(kind) = 'show' AND t.kind = 'show')
    OR (sqlc.narg(kind) = 'episode' AND w.episode_id IS NOT NULL))
  AND (sqlc.narg(title_id)::bigint IS NULL OR w.title_id = sqlc.narg(title_id) OR w.episode_id = sqlc.narg(title_id))
  AND (sqlc.narg(start_at)::timestamptz IS NULL OR w.watched_at >= sqlc.narg(start_at))
  AND (sqlc.narg(end_at)::timestamptz IS NULL OR w.watched_at <= sqlc.narg(end_at))
ORDER BY w.watched_at DESC, w.id DESC
LIMIT $2 OFFSET $3;

-- name: CountHistory :one
SELECT count(*)
FROM watch_events w
JOIN titles t ON t.id = w.title_id
WHERE w.profile_id = $1
  AND (sqlc.narg(kind)::text IS NULL
    OR (sqlc.narg(kind) = 'movie' AND t.kind = 'movie')
    OR (sqlc.narg(kind) = 'show' AND t.kind = 'show')
    OR (sqlc.narg(kind) = 'episode' AND w.episode_id IS NOT NULL))
  AND (sqlc.narg(title_id)::bigint IS NULL OR w.title_id = sqlc.narg(title_id) OR w.episode_id = sqlc.narg(title_id))
  AND (sqlc.narg(start_at)::timestamptz IS NULL OR w.watched_at >= sqlc.narg(start_at))
  AND (sqlc.narg(end_at)::timestamptz IS NULL OR w.watched_at <= sqlc.narg(end_at));

-- name: WatchEventExists :one
SELECT EXISTS (
    SELECT 1 FROM watch_events
    WHERE profile_id = $1 AND title_id = $2 AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id) AND watched_at = $3
);

-- name: DeleteWatchEventsByID :many
DELETE FROM watch_events WHERE profile_id = $1 AND id = ANY(@ids::bigint[])
RETURNING id, episode_id;

-- name: DeleteWatchEventsForItem :many
DELETE FROM watch_events w
WHERE w.profile_id = $1 AND w.title_id = $2
  AND (sqlc.narg(episode_id)::bigint IS NULL OR w.episode_id = sqlc.narg(episode_id))
  AND (sqlc.narg(season)::integer IS NULL
    OR w.episode_id IN (SELECT id FROM episodes WHERE show_id = $2 AND season_number = sqlc.narg(season)))
RETURNING w.id, w.episode_id;

-- name: SavePlayback :one
INSERT INTO playback (profile_id, title_id, episode_id, progress, paused_at, grant_id, duration_seconds)
VALUES ($1, $2, $3, $4, now(), $5, $6)
ON CONFLICT (profile_id, title_id, episode_id)
DO UPDATE SET progress = excluded.progress, paused_at = excluded.paused_at,
    grant_id = excluded.grant_id, duration_seconds = coalesce(excluded.duration_seconds, playback.duration_seconds)
RETURNING *;

-- name: DeletePlaybackForItem :execrows
DELETE FROM playback WHERE profile_id = $1 AND title_id = $2 AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id);

-- name: DeletePlayback :one
DELETE FROM playback WHERE id = $1 AND profile_id = $2
RETURNING episode_id;

-- name: ListPlayback :many
SELECT * FROM playback
WHERE profile_id = $1
  AND (sqlc.narg(kind)::text IS NULL
    OR (sqlc.narg(kind) = 'movie' AND episode_id IS NULL)
    OR (sqlc.narg(kind) = 'episode' AND episode_id IS NOT NULL))
  AND (sqlc.narg(start_at)::timestamptz IS NULL OR paused_at >= sqlc.narg(start_at))
  AND (sqlc.narg(end_at)::timestamptz IS NULL OR paused_at <= sqlc.narg(end_at))
ORDER BY paused_at DESC
LIMIT $2 OFFSET $3;

-- name: SetWatching :exec
INSERT INTO watching (profile_id, title_id, episode_id, grant_id, progress, started_at, expires_at)
VALUES ($1, $2, $3, $4, $5, now(), $6)
ON CONFLICT (profile_id) DO UPDATE SET title_id = excluded.title_id, episode_id = excluded.episode_id,
    grant_id = excluded.grant_id, progress = excluded.progress, started_at = excluded.started_at,
    expires_at = excluded.expires_at;

-- name: Watching :one
SELECT * FROM watching WHERE profile_id = $1 FOR UPDATE;

-- name: ClearWatching :exec
DELETE FROM watching WHERE profile_id = $1;

-- name: MarkActivity :exec
INSERT INTO activity_marks (profile_id, area, at) VALUES ($1, $2, now())
ON CONFLICT (profile_id, area) DO UPDATE SET at = excluded.at;

-- name: ActivityMarks :many
SELECT area, at FROM activity_marks WHERE profile_id = $1;

-- name: ProfileByID :one
SELECT * FROM profiles WHERE id = $1;

-- name: Watchlist :one
INSERT INTO lists (profile_id, kind, name, slug)
VALUES ($1, 'watchlist', 'Watchlist', 'watchlist')
ON CONFLICT (profile_id) WHERE kind = 'watchlist' DO UPDATE SET kind = lists.kind
RETURNING *;

-- name: ListItems :many
SELECT li.*, t.kind AS title_kind, s.season_number
FROM list_items li
JOIN titles t ON t.id = li.title_id
LEFT JOIN seasons s ON s.id = li.season_id
WHERE li.list_id = $1
  AND (sqlc.narg(kind)::text IS NULL
    OR (sqlc.narg(kind) = 'movie' AND t.kind = 'movie' AND li.episode_id IS NULL)
    OR (sqlc.narg(kind) = 'show' AND t.kind = 'show' AND li.season_id IS NULL AND li.episode_id IS NULL)
    OR (sqlc.narg(kind) = 'season' AND li.season_id IS NOT NULL AND li.episode_id IS NULL)
    OR (sqlc.narg(kind) = 'episode' AND li.episode_id IS NOT NULL))
ORDER BY li.rank, li.id;

-- name: AddListItem :one
INSERT INTO list_items (list_id, title_id, season_id, episode_id, rank, notes, listed_at)
SELECT $1, $2, $3, $4, coalesce(max(rank), 0) + 1, $5, coalesce(sqlc.narg(listed_at)::timestamptz, now())
FROM list_items WHERE list_id = $1
ON CONFLICT (list_id, title_id, season_id, episode_id) DO NOTHING
RETURNING *;

-- name: RemoveListItem :execrows
DELETE FROM list_items
WHERE list_id = $1 AND title_id = $2
  AND season_id IS NOT DISTINCT FROM sqlc.narg(season_id)
  AND episode_id IS NOT DISTINCT FROM sqlc.narg(episode_id);

-- name: CountListItems :one
SELECT count(*) FROM list_items WHERE list_id = $1;

-- name: TouchList :one
UPDATE lists SET updated_at = now() WHERE id = $1 RETURNING updated_at;

-- name: DeleteListItem :one
DELETE FROM list_items li
USING lists l
WHERE li.id = $1 AND l.id = li.list_id AND l.profile_id = $2 AND l.kind = 'watchlist'
RETURNING li.list_id, li.season_id, li.episode_id, (SELECT kind FROM titles WHERE id = li.title_id)::text AS kind;

-- name: EpisodePlayback :many
SELECT id, episode_id, progress FROM playback
WHERE profile_id = $1 AND episode_id = ANY(sqlc.arg(episode_ids)::bigint[]);

-- name: HideItem :exec
INSERT INTO hidden_items (profile_id, section, title_id, season_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: UnhideItem :execrows
DELETE FROM hidden_items
WHERE profile_id = $1 AND section = $2 AND title_id = $3 AND season_id IS NOT DISTINCT FROM sqlc.narg(season_id);

-- name: HiddenItems :many
SELECT h.*, s.season_number
FROM hidden_items h
LEFT JOIN seasons s ON s.id = h.season_id
WHERE h.profile_id = $1 AND h.section = $2
ORDER BY h.hidden_at DESC;

-- name: TitleWatches :many
SELECT id, episode_id, watched_at, precision, action FROM watch_events
WHERE profile_id = $1 AND title_id = $2
ORDER BY watched_at DESC, id DESC;

-- name: TitlePlayback :many
SELECT id, episode_id, progress::float8 AS progress FROM playback
WHERE profile_id = $1 AND title_id = $2;

-- name: TitleRating :one
SELECT id, rating, rated_at FROM ratings
WHERE profile_id = $1 AND title_id = $2 AND season_id IS NULL AND episode_id IS NULL;

-- name: TitleListing :one
SELECT i.id, i.listed_at
FROM list_items i
JOIN lists l ON l.id = i.list_id
WHERE l.profile_id = $1 AND l.kind = 'watchlist' AND i.title_id = $2 AND i.season_id IS NULL AND i.episode_id IS NULL;

-- name: WatchlistShows :many
SELECT DISTINCT i.title_id
FROM list_items i
JOIN lists l ON l.id = i.list_id
JOIN titles t ON t.id = i.title_id
WHERE l.profile_id = $1 AND l.kind = 'watchlist' AND t.kind = 'show';

-- name: StartRewatch :exec
INSERT INTO hidden_items (profile_id, section, title_id, hidden_at)
VALUES ($1, 'progress_watched_reset', $2, $3)
ON CONFLICT (profile_id, section, title_id, season_id) DO UPDATE SET hidden_at = excluded.hidden_at, import_id = NULL;
