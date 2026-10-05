-- name: CreateWallet :exec
INSERT INTO wallets (user_id, balance) VALUES ($1, $2);

-- name: GetBalance :one
SELECT balance FROM wallets WHERE user_id = $1;

-- name: LockWallet :one
-- Must run inside a transaction. Serialises all balance changes for a user.
SELECT * FROM wallets WHERE user_id = $1 FOR UPDATE;

-- name: SetBalance :exec
UPDATE wallets SET balance = $2, updated_at = now() WHERE user_id = $1;

-- name: InsertTransaction :exec
INSERT INTO wallet_transactions (user_id, kind, amount, balance_after, game_id, spin_id, bonus_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListTransactions :many
SELECT * FROM wallet_transactions
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(before)::bigint IS NULL OR id < sqlc.narg(before)::bigint)
ORDER BY id DESC
LIMIT sqlc.arg(lim);
