package games

import "pantheon-spins/server/internal/api"

// Symbol is an index into a game's symbol table. Using small integers keeps
// evaluation fast in the simulator; IDs are only used at the API boundary.
type Symbol uint8

// Empty marks a cell with no symbol (e.g. cleared during a cascade).
const Empty Symbol = 255

// Pos is a cell on the grid. Row 0 is the top of a reel.
type Pos struct {
	Reel, Row int
}

func (p Pos) API() api.Position { return api.Position{Reel: p.Reel, Row: p.Row} }

// PositionsAPI converts positions for the API.
func PositionsAPI(ps []Pos) []api.Position {
	out := make([]api.Position, len(ps))
	for i, p := range ps {
		out[i] = p.API()
	}
	return out
}

// Grid holds symbols by reel (column), each from top to bottom. Reels may have
// different heights (Megaways, honeycomb layouts).
type Grid [][]Symbol

// NewGrid returns an empty grid with the given reel heights.
func NewGrid(heights []int) Grid {
	g := make(Grid, len(heights))
	for r, h := range heights {
		g[r] = make([]Symbol, h)
		for i := range g[r] {
			g[r][i] = Empty
		}
	}
	return g
}

// Uniform returns heights for a rectangular reels × rows grid.
func Uniform(reels, rows int) []int {
	h := make([]int, reels)
	for i := range h {
		h[i] = rows
	}
	return h
}

func (g Grid) At(p Pos) Symbol { return g[p.Reel][p.Row] }

func (g Grid) Set(p Pos, s Symbol) { g[p.Reel][p.Row] = s }

// In reports whether p is on the grid.
func (g Grid) In(p Pos) bool {
	return p.Reel >= 0 && p.Reel < len(g) && p.Row >= 0 && p.Row < len(g[p.Reel])
}

// Size returns the number of cells.
func (g Grid) Size() int {
	n := 0
	for _, reel := range g {
		n += len(reel)
	}
	return n
}

// Clone returns a deep copy.
func (g Grid) Clone() Grid {
	c := make(Grid, len(g))
	for r, reel := range g {
		c[r] = append([]Symbol(nil), reel...)
	}
	return c
}

// Find returns every position whose symbol satisfies match, reel by reel.
func (g Grid) Find(match func(Symbol) bool) []Pos {
	var out []Pos
	for r, reel := range g {
		for row, s := range reel {
			if match(s) {
				out = append(out, Pos{r, row})
			}
		}
	}
	return out
}

// Count returns how many cells satisfy match.
func (g Grid) Count(match func(Symbol) bool) int {
	n := 0
	for _, reel := range g {
		for _, s := range reel {
			if match(s) {
				n++
			}
		}
	}
	return n
}

// Anticipation returns the reels to spin slowly because a feature needing
// `need` symbols could still land: every reel after the one where the count
// of matching symbols (scanning left to right) first reaches need-1.
// It returns nil when that never happens or the feature already landed early.
func Anticipation(g Grid, match func(Symbol) bool, need int) []int {
	count := 0
	for r, reel := range g {
		for _, s := range reel {
			if match(s) {
				count++
			}
		}
		if count >= need {
			return nil
		}
		if count == need-1 {
			if r == len(g)-1 {
				return nil
			}
			out := make([]int, 0, len(g)-r-1)
			for i := r + 1; i < len(g); i++ {
				out = append(out, i)
			}
			return out
		}
	}
	return nil
}
