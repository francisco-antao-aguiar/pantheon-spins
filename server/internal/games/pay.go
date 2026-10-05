package games

import (
	"fmt"
	"math"
)

// Units measure wins in twentieths of the bet (0.05×). Every bet level is a
// multiple of UnitsPerBet Coins, so converting units to Coins is exact and
// the simulator's RTP matches what players actually receive at any bet.
type Units int64

const UnitsPerBet = 20

// Coins converts a pay in units to Coins for a bet. bet must be a multiple of
// UnitsPerBet (enforced by config validation).
func (u Units) Coins(bet int64) int64 {
	return int64(u) * (bet / UnitsPerBet)
}

// UnitsFromMultiple converts a bet multiple from a config (e.g. 0.15) to units,
// failing if it is not a multiple of 0.05.
func UnitsFromMultiple(x float64) (Units, error) {
	u := x * UnitsPerBet
	r := math.Round(u)
	if x < 0 || math.Abs(u-r) > 1e-9 {
		return 0, fmt.Errorf("pay %v is not a non-negative multiple of %v×", x, 1.0/UnitsPerBet)
	}
	return Units(r), nil
}

// Paytable maps symbol and count (symbols in a line or way, or cluster size)
// to a pay in units. Lookups beyond the table use the last entry, so a
// cluster table defined up to 15 also pays 15+ clusters.
type Paytable [][]Units

// Pay returns the pay for count symbols s, or 0.
func (p Paytable) Pay(s Symbol, count int) Units {
	if int(s) >= len(p) || count <= 0 {
		return 0
	}
	row := p[s]
	if len(row) == 0 {
		return 0
	}
	if count >= len(row) {
		return row[len(row)-1]
	}
	return row[count]
}

// MinCount is the smallest count that pays for s, or 0 if s never pays.
func (p Paytable) MinCount(s Symbol) int {
	if int(s) >= len(p) {
		return 0
	}
	for n, u := range p[s] {
		if u > 0 {
			return n
		}
	}
	return 0
}

// Pays reports whether s pays at any count.
func (p Paytable) Pays(s Symbol) bool { return p.MinCount(s) > 0 }

// Win is one paying combination found by an evaluator.
type Win struct {
	Symbol    Symbol
	Positions []Pos
	// Pay is the final pay in units, including Multiplier.
	Pay Units
	// Count is symbols matched: reels for ways and lines, cells for clusters.
	Count int
	// Ways is the number of ways (ways games only).
	Ways int
	// Line is the 1-based payline number (line games only).
	Line int
	// Multiplier is the total multiplier applied (1 when none).
	Multiplier int
}

// TotalPay sums the pays of wins.
func TotalPay(wins []Win) Units {
	var t Units
	for _, w := range wins {
		t += w.Pay
	}
	return t
}

// MultiplyWins applies a multiplier to every win in place.
func MultiplyWins(wins []Win, m int) {
	if m <= 1 {
		return
	}
	for i := range wins {
		wins[i].Pay *= Units(m)
		wins[i].Multiplier *= m
	}
}

// WinPositions returns the distinct positions covered by wins.
func WinPositions(wins []Win) []Pos {
	seen := map[Pos]bool{}
	var out []Pos
	for _, w := range wins {
		for _, p := range w.Positions {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
