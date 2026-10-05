//go:build !devtools

package httpapi

import "github.com/go-chi/chi/v5"

// DevToolsEnabled reports whether this binary was built with -tags devtools.
const DevToolsEnabled = false

func (s *Server) devRoutes(chi.Router) {}
