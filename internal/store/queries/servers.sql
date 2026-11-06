-- name: CreateMediaServer :one
INSERT INTO media_servers (account_id, kind, secret_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: AccountMediaServers :many
SELECT * FROM media_servers
WHERE account_id = $1 AND revoked_at IS NULL
ORDER BY created_at;

-- name: MediaServerBySecret :one
SELECT * FROM media_servers
WHERE secret_hash = $1 AND kind = $2 AND revoked_at IS NULL;

-- name: SetMediaServerSecret :execrows
UPDATE media_servers SET secret_hash = $3
WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL;

-- name: RevokeMediaServer :execrows
UPDATE media_servers SET revoked_at = now()
WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL;

-- name: TouchMediaServer :exec
UPDATE media_servers SET name = coalesce(sqlc.narg(name), name), last_event_at = now()
WHERE id = $1;

-- name: SeeMediaServerUser :one
INSERT INTO media_server_users (server_id, external_id, name)
VALUES ($1, $2, $3)
ON CONFLICT (server_id, external_id) DO UPDATE SET name = excluded.name, last_event_at = now()
RETURNING *;

-- name: MediaServerUsers :many
SELECT u.*, g.profile_id
FROM media_server_users u
JOIN media_servers s ON s.id = u.server_id
LEFT JOIN oauth_grants g ON g.id = u.grant_id AND g.revoked_at IS NULL
WHERE s.account_id = $1 AND s.revoked_at IS NULL
ORDER BY u.server_id, u.name;

-- name: MediaServerUser :one
SELECT u.*, s.kind, s.name AS server_name
FROM media_server_users u
JOIN media_servers s ON s.id = u.server_id
WHERE u.id = $1 AND u.server_id = $2 AND s.account_id = $3 AND s.revoked_at IS NULL;

-- name: MatchMediaServerUser :exec
UPDATE media_server_users SET grant_id = $2 WHERE id = $1;

-- name: ServerUserGrant :one
SELECT g.id, g.profile_id
FROM oauth_grants g
WHERE g.id = $1 AND g.revoked_at IS NULL;

-- name: RevokeMediaServerGrants :exec
UPDATE oauth_grants SET revoked_at = now()
WHERE revoked_at IS NULL AND id IN (SELECT grant_id FROM media_server_users WHERE server_id = $1);

-- name: BuiltInApp :one
SELECT * FROM apps WHERE client_id = $1 AND built_in;
