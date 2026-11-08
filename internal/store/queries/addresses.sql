-- name: KeepOldUsername :exec
INSERT INTO old_usernames (username, profile_id)
VALUES ($1, $2)
ON CONFLICT (username) DO UPDATE SET profile_id = excluded.profile_id, changed_at = now();

-- name: ProfileByOldUsername :one
SELECT p.* FROM profiles p
JOIN old_usernames o ON o.profile_id = p.id
WHERE o.username = lower($1);

-- name: KeepOldListSlug :exec
INSERT INTO old_list_slugs (profile_id, slug, list_id)
VALUES ($1, $2, $3)
ON CONFLICT (profile_id, slug) DO UPDATE SET list_id = excluded.list_id, changed_at = now();

-- name: ListIDBySlug :one
SELECT a.id FROM (
    SELECT l.id, 0 AS age FROM lists l WHERE l.profile_id = $1 AND l.slug = $2
    UNION ALL
    SELECT o.list_id, 1 FROM old_list_slugs o WHERE o.profile_id = $1 AND o.slug = $2
) a
ORDER BY a.age
LIMIT 1;
