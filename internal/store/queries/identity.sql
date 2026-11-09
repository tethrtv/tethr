-- name: CreateAccount :one
INSERT INTO accounts (email, name)
VALUES ($1, $2)
RETURNING *;

-- name: CreateHousehold :one
INSERT INTO households (owner_account_id)
VALUES ($1)
RETURNING *;

-- name: CreateProfile :one
INSERT INTO profiles (household_id, account_id, username, name, kind, position)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: HouseholdOfAccount :one
SELECT * FROM households WHERE owner_account_id = $1;

-- name: NextProfilePosition :one
SELECT (coalesce(max(position), 0) + 1)::integer FROM profiles WHERE household_id = $1;

-- name: HouseholdProfile :one
SELECT p.*
FROM profiles p
JOIN households h ON h.id = p.household_id
WHERE p.id = $1 AND h.owner_account_id = $2;

-- name: UpdateProfile :one
UPDATE profiles
SET name = coalesce(sqlc.narg(name), name),
    username = coalesce(sqlc.narg(username), username),
    private = coalesce(sqlc.narg(private), private),
    in_popular = coalesce(sqlc.narg(in_popular), in_popular),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteProfile :exec
DELETE FROM profiles WHERE id = $1;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = $1;

-- name: OwnerProfile :one
SELECT p.*
FROM profiles p
JOIN households h ON h.id = p.household_id
WHERE h.owner_account_id = $1 AND p.kind = 'owner';
