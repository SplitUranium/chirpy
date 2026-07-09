-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: ResetUsers :exec
DELETE FROM users;

-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetAllChirps :many
SELECT *
FROM chirps
ORDER BY created_at ASC;

-- name: GetOneChirp :one
SELECT *
FROM chirps
WHERE id = $1;

-- name: UserIDFromEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    NULL
);

-- name: GetUserFromRefreshToken :one
SELECT *
FROM refresh_tokens
WHERE token = $1;

-- name: RevokeToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE token = $1;

-- name: UpdateUser :one
UPDATE users
SET email = $2,
    hashed_password = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteChirp :exec
DELETE from chirps
WHERE id = $1;

-- name: UpgradeChirpyRed :exec
UPDATE users
SET is_chirpy_red = TRUE,
    updated_at = NOW()
WHERE id = $1;
