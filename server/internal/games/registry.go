package games

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"pantheon-spins/server/internal/api"
)

// Registry holds the games the server offers. Games are added at startup.
type Registry struct {
	mu    sync.RWMutex
	games map[string]SlotGame
}

func NewRegistry() *Registry {
	return &Registry{games: map[string]SlotGame{}}
}

// Register adds a game. It fails on an empty or duplicate ID.
func (r *Registry) Register(g SlotGame) error {
	id := g.Info().Id
	if id == "" {
		return fmt.Errorf("games: game has no id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.games[id]; dup {
		return fmt.Errorf("games: duplicate game id %q", id)
	}
	r.games[id] = g
	return nil
}

// Get returns the game with the given ID.
func (r *Registry) Get(id string) (SlotGame, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.games[id]
	return g, ok
}

// List returns every game's info, sorted by name.
func (r *Registry) List() []api.GameInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]api.GameInfo, 0, len(r.games))
	for _, g := range r.games {
		out = append(out, g.Info())
	}
	slices.SortFunc(out, func(a, b api.GameInfo) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// ValidBet reports whether bet is one of the game's bet levels.
func ValidBet(info api.GameInfo, bet int64) bool {
	return slices.Contains(info.BetLevels, bet)
}

// WinTierFor picks the celebration level for a win.
func WinTierFor(bet, win int64) api.WinTier {
	switch {
	case win <= 0 || bet <= 0:
		return api.None
	case win >= 50*bet:
		return api.Epic
	case win >= 25*bet:
		return api.Mega
	case win >= 10*bet:
		return api.Big
	default:
		return api.Normal
	}
}
