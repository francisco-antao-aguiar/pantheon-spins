package games

import (
	"fmt"
	"sort"
)

// WeightTable draws symbols with fixed integer weights.
type WeightTable struct {
	symbols []Symbol
	cum     []int // cumulative weights
	total   int
}

// NewWeightTable builds a table from symbol weights. Zero weights are dropped.
func NewWeightTable(weights map[Symbol]int) (*WeightTable, error) {
	keys := make([]Symbol, 0, len(weights))
	for s := range weights {
		keys = append(keys, s)
	}
	// Deterministic order, so a scripted RNG gives reproducible draws in tests.
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	t := &WeightTable{}
	for _, s := range keys {
		w := weights[s]
		if w < 0 {
			return nil, fmt.Errorf("negative weight %d for symbol %d", w, s)
		}
		if w == 0 {
			continue
		}
		t.total += w
		t.symbols = append(t.symbols, s)
		t.cum = append(t.cum, t.total)
	}
	if t.total == 0 {
		return nil, fmt.Errorf("weight table is empty")
	}
	return t, nil
}

// Draw returns a random symbol.
func (t *WeightTable) Draw(rng RNG) Symbol {
	x := rng.IntN(t.total)
	i := sort.SearchInts(t.cum, x+1)
	return t.symbols[i]
}

// Total returns the sum of weights.
func (t *WeightTable) Total() int { return t.total }

// Weight returns the weight of s.
func (t *WeightTable) Weight(s Symbol) int {
	prev := 0
	for i, sym := range t.symbols {
		if sym == s {
			return t.cum[i] - prev
		}
		prev = t.cum[i]
	}
	return 0
}

// ReelSet holds one weight table per reel.
type ReelSet []*WeightTable

// Fill draws every cell of a grid with the given reel heights.
func (rs ReelSet) Fill(rng RNG, heights []int) Grid {
	g := NewGrid(heights)
	for r := range g {
		for row := range g[r] {
			g[r][row] = rs[r].Draw(rng)
		}
	}
	return g
}

// Weighted picks an index with probability proportional to weights[i].
// It panics if the weights sum to zero; configs are validated at load.
func Weighted(rng RNG, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	x := rng.IntN(total)
	for i, w := range weights {
		if x < w {
			return i
		}
		x -= w
	}
	panic("unreachable")
}
