// Package games defines the slot game contract shared by every game package,
// plus the registry, RNG and shared helpers. Game packages never touch the
// wallet or the database: they are pure functions of (RNG, bet, state).
package games

import (
	"encoding/json"
	"errors"

	"pantheon-spins/server/internal/api"
)

// SlotGame is implemented by every game. Implementations must be safe for
// concurrent use; per-call randomness comes only from the RNG argument.
type SlotGame interface {
	Info() api.GameInfo

	// Spin plays one paid base-game spin.
	Spin(rng RNG, bet int64) (SpinOutcome, error)

	// StartBonus builds the initial bonus state after a spin triggered it.
	StartBonus(rng RNG, bet int64, trigger api.BonusTrigger) (BonusState, error)

	// BonusAction advances the bonus by one player action. It returns
	// ErrInvalidAction when the action is not allowed in the current state.
	BonusAction(rng RNG, state BonusState, action Action) (BonusState, BonusStep, error)
}

// SpinOutcome is what a base-game spin produced.
type SpinOutcome struct {
	Outcome api.SpinOutcome
	// Trigger is set when the spin triggered the game's bonus.
	Trigger *api.BonusTrigger
}

// BonusState is a bonus state machine snapshot. It is persisted as JSON so a
// bonus can resume after a refresh or disconnect.
type BonusState struct {
	Kind api.BonusKind `json:"kind"`
	// Done is set once the bonus is resolved and no further actions apply.
	Done bool `json:"done"`
	// Actions the player may send next.
	Actions []api.BonusActionOption `json:"actions"`
	// Public is sent to the client as BonusState.data.
	Public map[string]any `json:"public"`
	// Private is the game's own state, including hidden values the client must
	// not see (e.g. what unpicked tiles contain). Never sent to the client.
	Private json.RawMessage `json:"private,omitempty"`
}

// Action is a player action within a bonus.
type Action struct {
	Name   string
	Choice string
}

// BonusStep is the visible result of one bonus action.
type BonusStep struct {
	// Win is the amount won by this action, credited immediately.
	Win int64
	// Outcome is set for actions that play a spin (free spins, respins).
	Outcome *api.SpinOutcome
	// Reveal describes what the action uncovered (pick contents, wheel segment…).
	Reveal map[string]any
}

// ErrInvalidAction means the action is not allowed in the bonus's current state.
var ErrInvalidAction = errors.New("invalid bonus action")
