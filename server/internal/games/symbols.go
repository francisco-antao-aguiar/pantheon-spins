package games

import "pantheon-spins/server/internal/api"

// SymbolKind classifies symbols for evaluation.
type SymbolKind string

const (
	KindLow     SymbolKind = "low"
	KindHigh    SymbolKind = "high"
	KindWild    SymbolKind = "wild"    // substitutes for paying symbols
	KindScatter SymbolKind = "scatter" // pays/triggers anywhere, never substituted
	KindSpecial SymbolKind = "special" // game-specific (orbs, pearls, mystery…), never substituted
)

func (k SymbolKind) Valid() bool {
	switch k {
	case KindLow, KindHigh, KindWild, KindScatter, KindSpecial:
		return true
	}
	return false
}

// SymbolDef describes one symbol.
type SymbolDef struct {
	ID   string
	Name string
	Kind SymbolKind
}

// Symbols is a game's symbol table.
type Symbols struct {
	defs []SymbolDef
	byID map[string]Symbol
}

func NewSymbols(defs []SymbolDef) *Symbols {
	s := &Symbols{defs: defs, byID: make(map[string]Symbol, len(defs))}
	for i, d := range defs {
		s.byID[d.ID] = Symbol(i)
	}
	return s
}

func (s *Symbols) Len() int { return len(s.defs) }

func (s *Symbols) Def(sym Symbol) SymbolDef { return s.defs[sym] }

// ID returns the symbol's string ID, or "" for Empty.
func (s *Symbols) ID(sym Symbol) string {
	if int(sym) >= len(s.defs) {
		return ""
	}
	return s.defs[sym].ID
}

// Lookup finds a symbol by ID.
func (s *Symbols) Lookup(id string) (Symbol, bool) {
	sym, ok := s.byID[id]
	return sym, ok
}

// MustLookup finds a symbol by ID and panics if it is missing. Use it only for
// IDs a validated config guarantees.
func (s *Symbols) MustLookup(id string) Symbol {
	sym, ok := s.byID[id]
	if !ok {
		panic("games: unknown symbol " + id)
	}
	return sym
}

func (s *Symbols) Kind(sym Symbol) SymbolKind {
	if int(sym) >= len(s.defs) {
		return ""
	}
	return s.defs[sym].Kind
}

func (s *Symbols) IsWild(sym Symbol) bool    { return s.Kind(sym) == KindWild }
func (s *Symbols) IsScatter(sym Symbol) bool { return s.Kind(sym) == KindScatter }

// IsPaying reports whether sym is a regular paying symbol (low or high).
func (s *Symbols) IsPaying(sym Symbol) bool {
	k := s.Kind(sym)
	return k == KindLow || k == KindHigh
}

// OfKind returns all symbols of the given kinds, in table order.
func (s *Symbols) OfKind(kinds ...SymbolKind) []Symbol {
	var out []Symbol
	for i, d := range s.defs {
		for _, k := range kinds {
			if d.Kind == k {
				out = append(out, Symbol(i))
				break
			}
		}
	}
	return out
}

// GridAPI converts a grid to symbol IDs for the API.
func (s *Symbols) GridAPI(g Grid) api.Grid {
	out := make(api.Grid, len(g))
	for r, reel := range g {
		out[r] = make([]string, len(reel))
		for i, sym := range reel {
			out[r][i] = s.ID(sym)
		}
	}
	return out
}

// WinsAPI converts wins to Coins at the given bet for the API.
func (s *Symbols) WinsAPI(bet int64, wins []Win) []api.Win {
	out := make([]api.Win, len(wins))
	for i, w := range wins {
		m := w.Multiplier
		if m <= 1 {
			m = 0 // omitted; clients treat a missing multiplier as 1
		}
		out[i] = api.Win{
			Symbol:     s.ID(w.Symbol),
			Positions:  PositionsAPI(w.Positions),
			Amount:     w.Pay.Coins(bet),
			Multiplier: m,
			Ways:       w.Ways,
			Line:       w.Line,
			Count:      w.Count,
		}
	}
	return out
}
