// Package catalog lists every slot game. Adding a game means writing its
// package under internal/games/ and adding its constructor here; the server,
// registry and simulator need no other changes.
package catalog

import (
	"fmt"

	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/eyeofra"
	"pantheon-spins/server/internal/games/valhalla"
)

// Factory builds a game from the game-config directory.
type Factory func(configDir string) (games.SlotGame, error)

// factories returns every game's constructor.
func factories() []Factory {
	return []Factory{
		valhalla.New,
		eyeofra.New,
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
