-- name: AccountByEmail :one
SELECT * FROM accounts WHERE email = $1;

-- name: CreateEmailLink :one
INSERT INTO email_links (email, account_id, purpose, token_hash, code_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: RecentEmailLinks :one
SELECT count(*) FROM email_links WHERE email = $1 AND created_at > $2;

-- name: EmailLinkByToken :one
SELECT * FROM email_links WHERE token_hash = $1 FOR UPDATE;

-- name: LatestEmailLink :one
SELECT * FROM email_links
WHERE email = $1 AND used_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE;

-- name: CountEmailLinkAttempt :exec
UPDATE email_links SET attempts = attempts + 1 WHERE id = $1;

-- name: UseEmailLink :exec
UPDATE email_links SET used_at = now() WHERE id = $1;

-- name: VerifyAccountEmail :exec
UPDATE accounts SET email_verified_at = coalesce(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (account_id, profile_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: SessionByToken :one
SELECT * FROM sessions WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1 AND last_seen_at < now() - interval '1 minute';

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: AccountByID :one
SELECT * FROM accounts WHERE id = $1;

-- name: HouseholdProfiles :many
SELECT p.*
FROM profiles p
JOIN households h ON h.id = p.household_id
WHERE h.owner_account_id = $1
ORDER BY p.position, p.id;

-- name: CreateInvite :one
INSERT INTO invites (code, note) VALUES ($1, $2) RETURNING *;

-- name: InviteByCode :one
SELECT * FROM invites WHERE code = $1 FOR UPDATE;

-- name: UseInvite :exec
UPDATE invites SET used_at = now(), used_by_account_id = $2 WHERE id = $1;

-- name: ListSessions :many
SELECT * FROM sessions
WHERE account_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY last_seen_at DESC;

-- name: RevokeSessionByID :execrows
UPDATE sessions SET revoked_at = now() WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL;

-- name: RevokeOtherSessions :exec
UPDATE sessions SET revoked_at = now() WHERE account_id = $1 AND id <> $2 AND revoked_at IS NULL;

-- name: CreatePasskey :one
INSERT INTO passkeys (account_id, credential_id, credential, name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPasskeys :many
SELECT * FROM passkeys WHERE account_id = $1 ORDER BY created_at;

-- name: PasskeyByCredentialID :one
SELECT * FROM passkeys WHERE credential_id = $1;

-- name: UsePasskey :exec
UPDATE passkeys SET credential = $2, last_used_at = now() WHERE id = $1;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id = $1 AND account_id = $2;

-- name: CreateChallenge :exec
INSERT INTO auth_challenges (token_hash, account_id, kind, session_data, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: TakeChallenge :one
DELETE FROM auth_challenges
WHERE token_hash = $1 AND kind = $2 AND expires_at > now()
RETURNING *;

-- name: UpdateAccountSettings :one
UPDATE accounts SET
  time_zone = coalesce(sqlc.narg(time_zone), time_zone),
  weekly_email = coalesce(sqlc.narg(weekly_email), weekly_email),
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: Unsubscribe :execrows
UPDATE accounts SET weekly_email = false, updated_at = now() WHERE unsubscribe_token = $1;

-- name: WeeklyRecipients :many
SELECT a.id AS account_id, a.email, a.time_zone, a.unsubscribe_token, a.weekly_sent_at, p.id AS profile_id
FROM accounts a
JOIN profiles p ON p.account_id = a.id AND p.kind = 'owner'
WHERE a.weekly_email AND a.email_verified_at IS NOT NULL
  AND (a.weekly_sent_at IS NULL OR a.weekly_sent_at < sqlc.arg(sent_before)::timestamptz)
ORDER BY a.id;

-- name: ReleaseWeekly :exec
UPDATE accounts SET weekly_sent_at = sqlc.narg(sent_at) WHERE id = $1;

-- name: ClaimWeekly :execrows
UPDATE accounts SET weekly_sent_at = now()
WHERE id = $1 AND weekly_email AND (weekly_sent_at IS NULL OR weekly_sent_at < sqlc.arg(sent_before)::timestamptz);
