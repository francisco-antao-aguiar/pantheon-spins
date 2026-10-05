package games

// Topology defines which cells are adjacent.
type Topology interface {
	// Neighbors appends the on-grid neighbours of p to buf and returns it.
	Neighbors(g Grid, p Pos, buf []Pos) []Pos
}

// Square is orthogonal adjacency on a rectangular (or ragged) grid.
type Square struct{}

func (Square) Neighbors(g Grid, p Pos, buf []Pos) []Pos {
	for _, d := range [4]Pos{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
		if n := (Pos{p.Reel + d.Reel, p.Row + d.Row}); g.In(n) {
			buf = append(buf, n)
		}
	}
	return buf
}

// Hex is a honeycomb of vertical columns ("odd-q" offset layout): odd columns
// sit half a cell lower than even ones. Each cell has up to six neighbours.
// Columns may differ in height to shape the board (e.g. 5,4,5,4,5).
type Hex struct{}

var (
	hexEven = [6]Pos{{0, -1}, {0, 1}, {-1, -1}, {-1, 0}, {1, -1}, {1, 0}}
	hexOdd  = [6]Pos{{0, -1}, {0, 1}, {-1, 0}, {-1, 1}, {1, 0}, {1, 1}}
)

func (Hex) Neighbors(g Grid, p Pos, buf []Pos) []Pos {
	deltas := &hexEven
	if p.Reel%2 == 1 {
		deltas = &hexOdd
	}
	for _, d := range deltas {
		if n := (Pos{p.Reel + d.Reel, p.Row + d.Row}); g.In(n) {
			buf = append(buf, n)
		}
	}
	return buf
}

// Clusters pays groups of adjacent identical symbols. Wilds join any cluster
// they touch (and can be part of clusters of different symbols), but a group
// of only wilds does not pay. Paytable counts are cluster sizes.
type Clusters struct {
	Symbols  *Symbols
	Pays     Paytable
	Topology Topology
}

// Evaluate returns one win per paying cluster. With cell multipliers, a
// cluster's pay is multiplied by the product of its cells' multipliers.
func (c Clusters) Evaluate(g Grid, mult CellMultiplier) []Win {
	var wins []Win
	visited := make([][]bool, len(g))
	for r := range g {
		visited[r] = make([]bool, len(g[r]))
	}
	var stack, nbuf []Pos

	for _, s := range c.Symbols.OfKind(KindLow, KindHigh) {
		minSize := c.Pays.MinCount(s)
		if minSize == 0 {
			continue
		}
		for r := range visited {
			clear(visited[r])
		}
		for r, reel := range g {
			for row, sym := range reel {
				if sym != s || visited[r][row] {
					continue
				}
				// Flood fill through s and wild cells.
				var cluster []Pos
				stack = append(stack[:0], Pos{r, row})
				visited[r][row] = true
				for len(stack) > 0 {
					p := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					cluster = append(cluster, p)
					nbuf = c.Topology.Neighbors(g, p, nbuf[:0])
					for _, n := range nbuf {
						if visited[n.Reel][n.Row] {
							continue
						}
						if ns := g.At(n); ns == s || c.Symbols.IsWild(ns) {
							visited[n.Reel][n.Row] = true
							stack = append(stack, n)
						}
					}
				}
				if len(cluster) < minSize {
					continue
				}
				m := 1
				for _, p := range cluster {
					m *= mult.at(p)
				}
				wins = append(wins, Win{
					Symbol:     s,
					Positions:  cluster,
					Pay:        c.Pays.Pay(s, len(cluster)) * Units(m),
					Count:      len(cluster),
					Multiplier: m,
				})
			}
		}
	}
	return wins
}
