-- name: FollowList :exec
INSERT INTO list_follows (profile_id, list_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UnfollowList :execrows
DELETE FROM list_follows WHERE profile_id = $1 AND list_id = $2;

-- name: FollowedListIDs :many
SELECT list_id FROM list_follows WHERE profile_id = $1 ORDER BY followed_at DESC;

-- name: ListFollowers :many
SELECT p.username, p.name, p.private, f.followed_at
FROM list_follows f
JOIN profiles p ON p.id = f.profile_id
WHERE f.list_id = $1
ORDER BY f.followed_at DESC, p.id;

-- name: BrowseLists :many
SELECT l.id, count(*) OVER ()::bigint AS total
FROM lists l
JOIN profiles o ON o.id = l.profile_id
LEFT JOIN (SELECT list_id, count(*) AS n FROM list_follows GROUP BY list_id) f ON f.list_id = l.id
LEFT JOIN (SELECT lf.list_id, count(*) AS n FROM list_follows lf WHERE lf.followed_at > sqlc.arg(since) GROUP BY lf.list_id) recent ON recent.list_id = l.id
WHERE l.kind = 'custom' AND l.privacy = 'public'
  AND EXISTS (SELECT 1 FROM list_items i WHERE i.list_id = l.id)
  AND NOT EXISTS (SELECT 1 FROM hidden_lists h WHERE h.list_id = l.id)
  AND (sqlc.narg(search)::text IS NULL OR l.name ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (sqlc.narg(only_official)::boolean IS NULL OR (o.username = sqlc.arg(official)::text) = sqlc.narg(only_official)::boolean)
  AND (NOT sqlc.arg(rising)::boolean OR recent.n > 0)
ORDER BY o.username = sqlc.arg(official)::text DESC,
  CASE WHEN sqlc.arg(rising)::boolean THEN coalesce(recent.n, 0) END DESC,
  CASE WHEN sqlc.arg(popular)::boolean THEN coalesce(f.n, 0) END DESC,
  l.created_at DESC, l.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: AddListReport :one
INSERT INTO list_reports (list_id, reporter_id, household_id, reason, note) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (list_id, household_id) WHERE resolved_at IS NULL DO NOTHING
RETURNING id;

-- name: CountReportsSince :one
SELECT count(*) FROM list_reports WHERE household_id = $1 AND created_at > $2;

-- name: ListHidden :one
SELECT EXISTS (SELECT 1 FROM hidden_lists WHERE list_id = $1);

-- name: OpenReports :many
SELECT r.id, r.list_id, r.reason, r.note, r.created_at, l.name AS list_name, l.slug AS list_slug, l.privacy,
    o.username AS owner_username, rp.username AS reporter_username,
    EXISTS (SELECT 1 FROM hidden_lists h WHERE h.list_id = r.list_id) AS hidden
FROM list_reports r
JOIN lists l ON l.id = r.list_id
JOIN profiles o ON o.id = l.profile_id
LEFT JOIN profiles rp ON rp.id = r.reporter_id
WHERE r.resolved_at IS NULL
ORDER BY r.list_id, r.created_at;

-- name: ResolveReports :execrows
UPDATE list_reports SET resolved_at = now(), outcome = $2
WHERE list_id = $1 AND resolved_at IS NULL;

-- name: MarkListKept :exec
UPDATE lists SET kept_at = now() WHERE id = $1;

-- name: ListOwnerContact :one
SELECT a.email, o.id AS profile_id, l.name
FROM lists l
JOIN profiles o ON o.id = l.profile_id
JOIN households h ON h.id = o.household_id
JOIN accounts a ON a.id = h.owner_account_id
WHERE l.id = $1;

-- name: MakeListPrivate :exec
UPDATE lists SET privacy = 'private', updated_at = now() WHERE id = $1;

-- name: PruneReports :exec
DELETE FROM list_reports WHERE resolved_at < now() - interval '1 year';
