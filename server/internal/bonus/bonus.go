// Package bonus persists bonus state machines in Postgres (JSONB) so an
// unresolved bonus survives refreshes and disconnects and resumes exactly
// where it stopped.
package bonus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/games"
)

// ErrNoActiveBonus means the user has no unresolved bonus on the game.
var ErrNoActiveBonus = errors.New("no active bonus")

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

// Session is a persisted bonus with its decoded state.
type Session struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	GameID   string
	Bet      int64
	TotalWin int64
	Step     int
	State    games.BonusState
}

func (s *Session) Status() string {
	if s.State.Done {
		return StatusCompleted
	}
	return StatusActive
}

// API returns the client-visible view. Private state is never included.
func (s *Session) API() api.BonusState {
	actions := s.State.Actions
	if s.State.Done || actions == nil {
		actions = []api.BonusActionOption{}
	}
	data := s.State.Public
	if data == nil {
		data = map[string]any{}
	}
	return api.BonusState{
		BonusId:  s.ID,
		GameId:   s.GameID,
		Kind:     s.State.Kind,
		Status:   api.BonusStatus(s.Status()),
		Bet:      s.Bet,
		TotalWin: s.TotalWin,
		Step:     s.Step,
		Actions:  actions,
		Data:     data,
	}
}

// Insert stores a newly started bonus. q must belong to the spin's transaction.
func Insert(ctx context.Context, q *db.Queries, s *Session, spinID uuid.UUID) error {
	raw, err := json.Marshal(s.State)
	if err != nil {
		return fmt.Errorf("bonus: encode state: %w", err)
	}
	err = q.InsertBonus(ctx, db.InsertBonusParams{
		ID:       s.ID,
		UserID:   s.UserID,
		GameID:   s.GameID,
		SpinID:   spinID,
		Kind:     string(s.State.Kind),
		Status:   s.Status(),
		Bet:      s.Bet,
		TotalWin: s.TotalWin,
		Step:     int32(s.Step),
		State:    raw,
	})
	if err != nil {
		return fmt.Errorf("bonus: insert: %w", err)
	}
	return nil
}

// Save writes back a bonus after an action.
func Save(ctx context.Context, q *db.Queries, s *Session) error {
	raw, err := json.Marshal(s.State)
	if err != nil {
		return fmt.Errorf("bonus: encode state: %w", err)
	}
	err = q.UpdateBonus(ctx, db.UpdateBonusParams{
		ID:       s.ID,
		Status:   s.Status(),
		TotalWin: s.TotalWin,
		Step:     int32(s.Step),
		State:    raw,
	})
	if err != nil {
		return fmt.Errorf("bonus: update: %w", err)
	}
	return nil
}

// Active loads the user's unresolved bonus on a game without locking it.
func Active(ctx context.Context, q *db.Queries, userID uuid.UUID, gameID string) (*Session, error) {
	row, err := q.GetActiveBonus(ctx, db.GetActiveBonusParams{UserID: userID, GameID: gameID})
	return fromRow(row, err)
}

// LockActive loads and row-locks the user's unresolved bonus on a game.
// Call it inside a transaction that already holds the wallet lock.
func LockActive(ctx context.Context, q *db.Queries, userID uuid.UUID, gameID string) (*Session, error) {
	row, err := q.LockActiveBonus(ctx, db.LockActiveBonusParams{UserID: userID, GameID: gameID})
	return fromRow(row, err)
}

func fromRow(row db.BonusSession, err error) (*Session, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoActiveBonus
	}
	if err != nil {
		return nil, fmt.Errorf("bonus: load: %w", err)
	}
	s := &Session{
		ID:       row.ID,
		UserID:   row.UserID,
		GameID:   row.GameID,
		Bet:      row.Bet,
		TotalWin: row.TotalWin,
		Step:     int(row.Step),
	}
	if err := json.Unmarshal(row.State, &s.State); err != nil {
		return nil, fmt.Errorf("bonus: decode state %s: %w", row.ID, err)
	}
	return s, nil
}
