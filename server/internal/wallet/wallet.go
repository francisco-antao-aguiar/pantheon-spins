// Package wallet owns every change to a user's Coin balance. All changes run
// in one Postgres transaction that holds the wallet row lock (SELECT … FOR
// UPDATE), so parallel requests for the same user are serialised.
package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/db"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Ref links a transaction to what caused it.
type Ref struct {
	GameID  string
	SpinID  *uuid.UUID
	BonusID *uuid.UUID
}

// Locked is a wallet whose row is locked for the current transaction.
// Use it only inside the callback passed to WithLock.
type Locked struct {
	ctx     context.Context
	Q       *db.Queries
	userID  uuid.UUID
	balance int64
	dirty   bool
}

func (l *Locked) UserID() uuid.UUID { return l.userID }
func (l *Locked) Balance() int64    { return l.balance }

// Apply changes the balance by amount (negative for debits) and records a
// ledger entry. A zero amount is a no-op. It fails with ErrInsufficientFunds
// rather than letting the balance go negative.
func (l *Locked) Apply(kind api.TransactionKind, amount int64, ref Ref) error {
	if amount == 0 {
		return nil
	}
	next := l.balance + amount
	if amount < 0 && next < 0 {
		return ErrInsufficientFunds
	}
	if amount > 0 && next < l.balance {
		return fmt.Errorf("wallet: balance overflow")
	}
	var gameID *string
	if ref.GameID != "" {
		gameID = &ref.GameID
	}
	err := l.Q.InsertTransaction(l.ctx, db.InsertTransactionParams{
		UserID:       l.userID,
		Kind:         string(kind),
		Amount:       amount,
		BalanceAfter: next,
		GameID:       gameID,
		SpinID:       ref.SpinID,
		BonusID:      ref.BonusID,
	})
	if err != nil {
		return fmt.Errorf("wallet: insert transaction: %w", err)
	}
	l.balance = next
	l.dirty = true
	return nil
}

// WithLock runs fn in a transaction holding the user's wallet row lock. If fn
// returns an error, every change (including fn's own queries through l.Q) is
// rolled back.
func (s *Service) WithLock(ctx context.Context, userID uuid.UUID, fn func(l *Locked) error) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := db.New(tx)
		w, err := q.LockWallet(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWalletNotFound
		}
		if err != nil {
			return fmt.Errorf("wallet: lock: %w", err)
		}
		l := &Locked{ctx: ctx, Q: q, userID: userID, balance: w.Balance}
		if err := fn(l); err != nil {
			return err
		}
		if l.dirty {
			if err := q.SetBalance(ctx, db.SetBalanceParams{UserID: userID, Balance: l.balance}); err != nil {
				return fmt.Errorf("wallet: set balance: %w", err)
			}
		}
		return nil
	})
}

// Open creates a wallet with a starting grant. It must run inside the
// transaction that creates the user, through q.
func Open(ctx context.Context, q *db.Queries, userID uuid.UUID, grant int64) error {
	if err := q.CreateWallet(ctx, db.CreateWalletParams{UserID: userID, Balance: grant}); err != nil {
		return fmt.Errorf("wallet: create: %w", err)
	}
	if grant == 0 {
		return nil
	}
	return q.InsertTransaction(ctx, db.InsertTransactionParams{
		UserID:       userID,
		Kind:         string(api.SignupBonus),
		Amount:       grant,
		BalanceAfter: grant,
	})
}

func (s *Service) Balance(ctx context.Context, userID uuid.UUID) (int64, error) {
	b, err := db.New(s.pool).GetBalance(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrWalletNotFound
	}
	return b, err
}

// Transactions returns up to limit ledger entries, newest first, with IDs below before (if set).
func (s *Service) Transactions(ctx context.Context, userID uuid.UUID, before *int64, limit int) ([]db.WalletTransaction, error) {
	return db.New(s.pool).ListTransactions(ctx, db.ListTransactionsParams{
		UserID: userID,
		Before: before,
		Lim:    int32(limit),
	})
}
