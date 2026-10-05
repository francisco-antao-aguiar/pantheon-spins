-- name: InsertSpin :exec
INSERT INTO spins (id, user_id, game_id, bet, win, bonus_id, result)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListSpins :many
SELECT sp.id, sp.game_id, sp.bet, sp.win, sp.bonus_id, sp.created_at FROM spins sp
WHERE sp.user_id = sqlc.arg(user_id)::uuid
  AND (sqlc.narg(before)::uuid IS NULL OR (sp.created_at, sp.id) < (
        SELECT c.created_at, c.id FROM spins c
        WHERE c.id = sqlc.narg(before)::uuid AND c.user_id = sqlc.arg(user_id)::uuid))
ORDER BY sp.created_at DESC, sp.id DESC
LIMIT sqlc.arg(lim);

-- name: SumSpins :one
-- Totals used by consistency tests and stats.
SELECT count(*)::bigint AS spins, coalesce(sum(bet), 0)::bigint AS bets, coalesce(sum(win), 0)::bigint AS wins
FROM spins WHERE user_id = $1;
