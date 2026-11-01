-- name: CreateAccount :one
INSERT INTO accounts (email, name)
VALUES ($1, $2)
RETURNING *;

-- name: CreateHousehold :one
INSERT INTO households (owner_account_id)
VALUES ($1)
RETURNING *;

-- name: CreateProfile :one
INSERT INTO profiles (household_id, account_id, username, name, kind)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: OwnerProfile :one
SELECT p.*
FROM profiles p
JOIN households h ON h.id = p.household_id
WHERE h.owner_account_id = $1 AND p.kind = 'owner';
