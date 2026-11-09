-- name: PopularTitles :many
SELECT w.title_id, count(DISTINCT p.household_id)::int AS households
FROM watch_events w
JOIN profiles p ON p.id = w.profile_id
JOIN households h ON h.id = p.household_id
WHERE w.watched_at >= sqlc.arg(since) AND w.watched_at <= now() AND w.precision <> 'none'
  AND p.kind IN ('owner', 'person') AND p.in_popular
  AND h.created_at <= sqlc.arg(settled)
GROUP BY w.title_id
HAVING count(DISTINCT p.household_id) >= sqlc.arg(floor)::int
ORDER BY households DESC, w.title_id
LIMIT sqlc.arg(max_titles);
