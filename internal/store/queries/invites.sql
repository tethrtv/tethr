-- name: CreateUserInvite :one
INSERT INTO invites (code, created_by_account_id) VALUES ($1, $2) RETURNING *;

-- name: InvitesMadeBy :many
SELECT i.*, p.username AS used_by_username
FROM invites i
LEFT JOIN households h ON h.owner_account_id = i.used_by_account_id
LEFT JOIN profiles p ON p.household_id = h.id AND p.kind = 'owner'
WHERE i.created_by_account_id = $1
ORDER BY i.created_at, i.id;

-- name: ExtraInvites :one
SELECT extra_invites FROM accounts WHERE id = $1 FOR UPDATE;

-- name: GrantInvites :one
UPDATE accounts SET extra_invites = extra_invites + sqlc.arg(n)::int WHERE id = $1
RETURNING extra_invites;

-- name: CheckInvite :one
SELECT i.used_at, p.username AS invited_by
FROM invites i
LEFT JOIN households h ON h.owner_account_id = i.created_by_account_id
LEFT JOIN profiles p ON p.household_id = h.id AND p.kind = 'owner'
WHERE i.code = $1;

-- name: AccountOfUsername :one
SELECT h.owner_account_id
FROM profiles p
JOIN households h ON h.id = p.household_id
WHERE p.username = lower($1);

-- name: SwitchOn :one
SELECT EXISTS (SELECT 1 FROM switches WHERE name = $1);

-- name: TurnOn :exec
INSERT INTO switches (name) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: TurnOff :exec
DELETE FROM switches WHERE name = $1;
