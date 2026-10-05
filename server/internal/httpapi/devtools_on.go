//go:build devtools

package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// DevToolsEnabled reports whether this binary was built with -tags devtools.
const DevToolsEnabled = true

// devRoutes adds development-only endpoints. They are not in the OpenAPI spec
// and are compiled only into devtools builds (local Compose and E2E tests).
func (s *Server) devRoutes(r chi.Router) {
	r.Get("/dev/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"devtools": true})
	})
	// Plays a paid spin that triggers the bonus (at the game's default bet).
	r.Post("/dev/force-bonus/{gameId}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.requireUser(w, r)
		if !ok {
			return
		}
		gameID := chi.URLParam(r, "gameId")
		game, ok := s.Games.Get(gameID)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown_game", "No such game.")
			return
		}
		res, err := s.Play.ForceBonus(r.Context(), id, gameID, game.Info().DefaultBet)
		if err != nil {
			s.playError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}
