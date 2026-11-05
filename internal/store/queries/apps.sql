-- name: CreateApp :one
INSERT INTO apps (owner_account_id, client_id, client_secret_hash, name, redirect_uris)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: AccountApps :many
SELECT a.*,
    (SELECT count(*) FROM oauth_grants g WHERE g.app_id = a.id AND g.revoked_at IS NULL) AS connections
FROM apps a
WHERE a.owner_account_id = $1 AND a.revoked_at IS NULL
ORDER BY a.created_at DESC;

-- name: CountAccountApps :one
SELECT count(*) FROM apps WHERE owner_account_id = $1 AND revoked_at IS NULL;

-- name: UpdateApp :one
UPDATE apps SET name = coalesce(sqlc.narg(name), name), redirect_uris = coalesce(sqlc.narg(redirect_uris)::text[], redirect_uris)
WHERE id = $1 AND owner_account_id = $2 AND revoked_at IS NULL
RETURNING *;

-- name: RevokeApp :execrows
UPDATE apps SET revoked_at = now() WHERE id = $1 AND owner_account_id = $2 AND revoked_at IS NULL;

-- name: SetAppSecret :execrows
UPDATE apps SET client_secret_hash = $3 WHERE id = $1 AND owner_account_id = $2 AND revoked_at IS NULL;

-- name: AppByClientID :one
SELECT * FROM apps WHERE client_id = $1 AND revoked_at IS NULL;

-- name: CreateDeviceCode :one
INSERT INTO device_codes (app_id, device_code_hash, user_code, expires_at, interval_seconds)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeviceCodeByHash :one
SELECT * FROM device_codes WHERE device_code_hash = $1 FOR UPDATE;

-- name: PendingDeviceCode :one
SELECT d.*, a.name AS app_name
FROM device_codes d
JOIN apps a ON a.id = d.app_id
WHERE d.user_code = $1 AND d.status = 'pending' AND d.expires_at > now()
FOR UPDATE OF d;

-- name: PollDeviceCode :exec
UPDATE device_codes SET last_polled_at = now() WHERE id = $1;

-- name: SetDeviceCodeStatus :exec
UPDATE device_codes SET status = $2, profile_id = $3 WHERE id = $1;

-- name: CreateGrant :one
INSERT INTO oauth_grants (app_id, profile_id, label, user_agent, approved_via)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateToken :one
INSERT INTO oauth_tokens (grant_id, access_token_hash, refresh_token_hash, access_expires_at, refresh_expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: TokenByAccess :one
SELECT t.grant_id, g.profile_id, g.app_id, g.last_used_at, h.owner_account_id AS account_id
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
JOIN apps a ON a.id = g.app_id
JOIN profiles p ON p.id = g.profile_id
JOIN households h ON h.id = p.household_id
WHERE t.access_token_hash = $1
  AND t.revoked_at IS NULL AND t.access_expires_at > now()
  AND g.revoked_at IS NULL AND a.revoked_at IS NULL;

-- name: TokenByRefresh :one
SELECT t.*
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
WHERE t.refresh_token_hash = $1 AND g.app_id = $2
  AND t.revoked_at IS NULL AND t.refresh_expires_at > now()
  AND g.revoked_at IS NULL
FOR UPDATE OF t;

-- name: RevokeToken :exec
UPDATE oauth_tokens SET revoked_at = now() WHERE id = $1;

-- name: RevokeTokenByHash :execrows
UPDATE oauth_tokens t SET revoked_at = now()
FROM oauth_grants g
WHERE g.id = t.grant_id AND g.app_id = $2
  AND (t.access_token_hash = $1 OR t.refresh_token_hash = $1)
  AND t.revoked_at IS NULL;

-- name: TouchGrant :exec
UPDATE oauth_grants SET last_used_at = now()
WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (app_id, profile_id, code_hash, redirect_uri, code_challenge, code_challenge_method, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: TakeOAuthCode :one
UPDATE oauth_codes SET used_at = now()
WHERE code_hash = $1 AND app_id = $2 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: HouseholdGrants :many
SELECT g.*, a.name AS app_name, p.name AS profile_name
FROM oauth_grants g
JOIN apps a ON a.id = g.app_id
JOIN profiles p ON p.id = g.profile_id
JOIN households h ON h.id = p.household_id
WHERE h.owner_account_id = $1 AND g.revoked_at IS NULL
ORDER BY coalesce(g.last_used_at, g.created_at) DESC;

-- name: RevokeGrant :execrows
UPDATE oauth_grants g SET revoked_at = now()
FROM profiles p, households h
WHERE g.id = $1 AND p.id = g.profile_id AND h.id = p.household_id
  AND h.owner_account_id = $2 AND g.revoked_at IS NULL;

-- name: RevokeGrantTokens :exec
UPDATE oauth_tokens SET revoked_at = now() WHERE grant_id = $1 AND revoked_at IS NULL;

-- name: RenameGrant :execrows
UPDATE oauth_grants g SET label = $3
FROM profiles p, households h
WHERE g.id = $1 AND p.id = g.profile_id AND h.id = p.household_id
  AND h.owner_account_id = $2 AND g.revoked_at IS NULL;
