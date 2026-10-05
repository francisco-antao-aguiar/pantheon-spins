-- name: CreateUser :one
INSERT INTO users (email, username, password_hash, google_sub, avatar)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByGoogleSub :one
SELECT * FROM users WHERE google_sub = $1;

-- name: LinkGoogleAccount :exec
UPDATE users SET google_sub = $2, updated_at = now() WHERE id = $1;

-- name: UsernameExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE username = $1);

-- name: UpdateProfile :one
UPDATE users
SET username   = coalesce(sqlc.narg(username), username),
    avatar     = coalesce(sqlc.narg(avatar), avatar),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetPasswordHash :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: InvalidatePasswordResetTokens :exec
UPDATE password_reset_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL;

-- name: ConsumePasswordResetToken :one
-- Marks a valid token used and returns its user. No row means invalid, expired or used.
UPDATE password_reset_tokens
SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING user_id;
