// Package catalog lists every slot game. Adding a game means writing its
// package under internal/games/ and adding its constructor here; the server,
// registry and simulator need no other changes.
package catalog

import (
	"fmt"

	"pantheon-spins/server/internal/games"
)

// Factory builds a game from the game-config directory.
type Factory func(configDir string) (games.SlotGame, error)

// factories returns every game's constructor.
func factories() []Factory {
	return []Factory{
		// Games are added here as they are built (Halls of Valhalla is next).
	}
}

// Register builds every game and adds it to reg.
func Register(reg *games.Registry, configDir string) error {
	for _, f := range factories() {
		g, err := f(configDir)
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		if err := reg.Register(g); err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
	}
	return nil
}
