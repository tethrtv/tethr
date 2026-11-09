-- name: YearPlays :many
SELECT w.id, w.title_id, w.episode_id, w.watched_at, w.precision, w.action, w.duration_seconds,
  EXISTS (
    SELECT 1 FROM watch_events p
    WHERE p.profile_id = w.profile_id AND p.title_id = w.title_id
      AND p.episode_id IS NOT DISTINCT FROM w.episode_id
      AND (p.watched_at, p.id) < (w.watched_at, w.id)
  )::boolean AS again,
  (SELECT min(p.watched_at) FROM watch_events p WHERE p.profile_id = w.profile_id AND p.title_id = w.title_id)::timestamptz AS first_seen
FROM watch_events w
WHERE w.profile_id = $1 AND w.precision <> 'none'
  AND w.watched_at >= sqlc.arg(start_at) AND w.watched_at < sqlc.arg(end_at)
ORDER BY w.watched_at, w.id;

-- name: PlayYears :many
SELECT DISTINCT extract(year FROM watched_at AT TIME ZONE 'UTC')::int AS year
FROM watch_events
WHERE profile_id = $1 AND precision <> 'none'
ORDER BY year DESC;
