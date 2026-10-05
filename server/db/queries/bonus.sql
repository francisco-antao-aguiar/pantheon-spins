-- name: GetActiveBonus :one
SELECT * FROM bonus_sessions
WHERE user_id = $1 AND game_id = $2 AND status = 'active';

-- name: LockActiveBonus :one
-- Must run inside a transaction, after locking the wallet.
SELECT * FROM bonus_sessions
WHERE user_id = $1 AND game_id = $2 AND status = 'active'
FOR UPDATE;

-- name: InsertBonus :exec
INSERT INTO bonus_sessions (id, user_id, game_id, spin_id, kind, status, bet, total_win, step, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpdateBonus :exec
UPDATE bonus_sessions
SET status       = $2,
    total_win    = $3,
    step         = $4,
    state        = $5,
    updated_at   = now(),
    completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE NULL END
WHERE id = $1;

-- name: SumBonusWins :one
SELECT coalesce(sum(total_win), 0)::bigint FROM bonus_sessions WHERE user_id = $1;
