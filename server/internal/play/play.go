// Package play runs paid spins and bonus actions. Each call is one Postgres
// transaction that holds the wallet lock: debit bet, compute the outcome with
// crypto randomness, credit the win, write the spin log and persist any bonus.
package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/bonus"
	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/wallet"
)

var (
	ErrUnknownGame     = errors.New("unknown game")
	ErrInvalidBet      = errors.New("invalid bet")
	ErrBonusInProgress = errors.New("bonus in progress")
	ErrStaleStep       = errors.New("stale bonus step")
)

type Service struct {
	pool   *pgxpool.Pool
	wallet *wallet.Service
	games  *games.Registry
	newRNG func() games.RNG
}

func NewService(pool *pgxpool.Pool, w *wallet.Service, reg *games.Registry) *Service {
	return &Service{
		pool:   pool,
		wallet: w,
		games:  reg,
		newRNG: func() games.RNG { return games.NewCryptoRNG() },
	}
}

// Spin plays one paid spin of gameID for the user.
func (s *Service) Spin(ctx context.Context, userID uuid.UUID, gameID string, bet int64) (api.SpinResult, error) {
	game, ok := s.games.Get(gameID)
	if !ok {
		return api.SpinResult{}, ErrUnknownGame
	}
	if !games.ValidBet(game.Info(), bet) {
		return api.SpinResult{}, ErrInvalidBet
	}

	var res api.SpinResult
	err := s.wallet.WithLock(ctx, userID, func(l *wallet.Locked) error {
		// The wallet lock serialises this check with bonus creation.
		_, err := bonus.Active(ctx, l.Q, userID, gameID)
		switch {
		case err == nil:
			return ErrBonusInProgress
		case !errors.Is(err, bonus.ErrNoActiveBonus):
			return err
		}
		if l.Balance() < bet {
			return wallet.ErrInsufficientFunds
		}

		rng := s.newRNG()
		out, err := game.Spin(rng, bet)
		if err != nil {
			return fmt.Errorf("play: %s spin: %w", gameID, err)
		}
		win := out.Outcome.TotalWin
		if win < 0 {
			return fmt.Errorf("play: %s returned negative win %d", gameID, win)
		}

		spinID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		ref := wallet.Ref{GameID: gameID, SpinID: &spinID}

		var sess *bonus.Session
		if out.Trigger != nil {
			state, err := game.StartBonus(rng, bet, *out.Trigger)
			if err != nil {
				return fmt.Errorf("play: %s start bonus: %w", gameID, err)
			}
			bonusID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			sess = &bonus.Session{ID: bonusID, UserID: userID, GameID: gameID, Bet: bet, State: state}
		}

		res = api.SpinResult{
			SpinId:       spinID,
			GameId:       gameID,
			Bet:          bet,
			Outcome:      out.Outcome,
			TotalWin:     win,
			WinTier:      games.WinTierFor(bet, win),
			Balance:      l.Balance() - bet + win,
		}
		if sess != nil {
			res.BonusTrigger = *out.Trigger
			res.Bonus = sess.API()
		}

		raw, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("play: encode spin: %w", err)
		}
		var bonusID *uuid.UUID
		if sess != nil {
			bonusID = &sess.ID
		}
		// The spin row goes first: ledger entries and the bonus reference it.
		err = l.Q.InsertSpin(ctx, db.InsertSpinParams{
			ID: spinID, UserID: userID, GameID: gameID, Bet: bet, Win: win, BonusID: bonusID, Result: raw,
		})
		if err != nil {
			return fmt.Errorf("play: insert spin: %w", err)
		}
		if err := l.Apply(api.SpinBet, -bet, ref); err != nil {
			return err
		}
		if err := l.Apply(api.SpinWin, win, ref); err != nil {
			return err
		}
		if sess != nil {
			if err := bonus.Insert(ctx, l.Q, sess, spinID); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

// ActiveBonus returns the user's unresolved bonus on a game, or
// bonus.ErrNoActiveBonus.
func (s *Service) ActiveBonus(ctx context.Context, userID uuid.UUID, gameID string) (api.BonusState, error) {
	if _, ok := s.games.Get(gameID); !ok {
		return api.BonusState{}, ErrUnknownGame
	}
	sess, err := bonus.Active(ctx, db.New(s.pool), userID, gameID)
	if err != nil {
		return api.BonusState{}, err
	}
	return sess.API(), nil
}

// BonusAction applies one player action to the user's active bonus on a game.
// step must equal the bonus's current step, which makes retries and
// double-submits safe: the second one gets ErrStaleStep.
func (s *Service) BonusAction(ctx context.Context, userID uuid.UUID, gameID string, step int, action games.Action) (api.BonusStepResult, error) {
	game, ok := s.games.Get(gameID)
	if !ok {
		return api.BonusStepResult{}, ErrUnknownGame
	}

	var res api.BonusStepResult
	err := s.wallet.WithLock(ctx, userID, func(l *wallet.Locked) error {
		sess, err := bonus.LockActive(ctx, l.Q, userID, gameID)
		if err != nil {
			return err
		}
		if sess.Step != step {
			return ErrStaleStep
		}
		next, out, err := game.BonusAction(s.newRNG(), sess.State, action)
		if err != nil {
			return err
		}
		if out.Win < 0 {
			return fmt.Errorf("play: %s bonus returned negative win %d", gameID, out.Win)
		}

		sess.State = next
		sess.Step++
		sess.TotalWin += out.Win
		if err := bonus.Save(ctx, l.Q, sess); err != nil {
			return err
		}
		if err := l.Apply(api.BonusWin, out.Win, wallet.Ref{GameID: gameID, BonusID: &sess.ID}); err != nil {
			return err
		}

		res = api.BonusStepResult{
			State:   sess.API(),
			StepWin: out.Win,
			Balance: l.Balance(),
			Reveal:  out.Reveal,
		}
		if out.Outcome != nil {
			res.Outcome = *out.Outcome
		}
		return nil
	})
	return res, err
}

// History returns up to limit of the user's spins, newest first, starting
// after the spin with ID before (if set).
func (s *Service) History(ctx context.Context, userID uuid.UUID, before *uuid.UUID, limit int) ([]api.SpinHistoryItem, error) {
	rows, err := db.New(s.pool).ListSpins(ctx, db.ListSpinsParams{UserID: userID, Before: before, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	items := make([]api.SpinHistoryItem, len(rows))
	for i, r := range rows {
		items[i] = api.SpinHistoryItem{
			SpinId:         r.ID,
			GameId:         r.GameID,
			Bet:            r.Bet,
			Win:            r.Win,
			BonusTriggered: r.BonusID != nil,
			CreatedAt:      r.CreatedAt,
		}
	}
	return items, nil
}
