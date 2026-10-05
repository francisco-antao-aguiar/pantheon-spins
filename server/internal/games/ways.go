package games

// CellMultiplier returns the multiplier on a cell (1 when none). Evaluators
// take nil to mean "no multipliers".
type CellMultiplier func(Pos) int

func (m CellMultiplier) at(p Pos) int {
	if m == nil {
		return 1
	}
	if v := m(p); v > 1 {
		return v
	}
	return 1
}

// Ways evaluates "ways to win": a paying symbol pays when it (or a wild)
// appears on adjacent reels starting from the leftmost. Every combination of
// one matching cell per reel is a way. Reels may differ in height, so this
// also evaluates Megaways grids.
type Ways struct {
	Symbols *Symbols
	Pays    Paytable
}

// Evaluate returns one win per paying symbol. With cell multipliers, each way
// pays the product of the multipliers on its cells; summed over all ways that
// is the product, per reel, of the sum of matching cells' multipliers.
func (w Ways) Evaluate(g Grid, mult CellMultiplier) []Win {
	var wins []Win
	for _, s := range w.Symbols.OfKind(KindLow, KindHigh) {
		minCount := w.Pays.MinCount(s)
		if minCount == 0 {
			continue
		}
		ways, weighted := 1, 1
		var positions []Pos
		reels := 0
		for r, reel := range g {
			count, sum := 0, 0
			for row, sym := range reel {
				if sym == s || w.Symbols.IsWild(sym) {
					p := Pos{r, row}
					count++
					sum += mult.at(p)
					positions = append(positions, p)
				}
			}
			if count == 0 {
				break
			}
			ways *= count
			weighted *= sum
			reels++
		}
		pay := w.Pays.Pay(s, reels)
		if reels < minCount || pay == 0 {
			continue
		}
		// Report the effective multiplier when it is a whole number, so the
		// client can show "pay x ways x m".
		m := 1
		if weighted%ways == 0 {
			m = weighted / ways
		}
		wins = append(wins, Win{
			Symbol:     s,
			Positions:  positions,
			Pay:        pay * Units(weighted),
			Count:      reels,
			Ways:       ways,
			Multiplier: m,
		})
	}
	return wins
}
