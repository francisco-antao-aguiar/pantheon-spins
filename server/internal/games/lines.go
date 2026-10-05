package games

// Lines evaluates fixed paylines, left to right from the first reel. Each line
// lists the row used on every reel.
type Lines struct {
	Symbols *Symbols
	Pays    Paytable
	Lines   [][]int
}

// Evaluate returns at most one win per line: the best of the substituted
// symbol and, if wilds pay on their own, the leading run of wilds. With cell
// multipliers, a line pays the product of the multipliers on its matched cells.
func (l Lines) Evaluate(g Grid, mult CellMultiplier) []Win {
	var wins []Win
	for i, line := range l.Lines {
		cells := make([]Pos, len(line))
		for r, row := range line {
			cells[r] = Pos{r, row}
		}

		// The line's symbol is the first non-wild symbol.
		target := Empty
		for _, p := range cells {
			if s := g.At(p); !l.Symbols.IsWild(s) {
				target = s
				break
			}
		}

		best := l.lineWin(g, cells, target, mult)
		if lead := g.At(cells[0]); l.Symbols.IsWild(lead) && l.Pays.Pays(lead) {
			if w := l.lineWin(g, cells, lead, mult); w.Pay > best.Pay {
				best = w
			}
		}
		if best.Pay > 0 {
			best.Line = i + 1
			wins = append(wins, best)
		}
	}
	return wins
}

// lineWin counts the run from reel 0 of target (or wilds substituting for it).
// When target is a wild, only that wild counts (a wild-only run).
func (l Lines) lineWin(g Grid, cells []Pos, target Symbol, mult CellMultiplier) Win {
	if target == Empty || l.Symbols.IsScatter(target) || l.Symbols.Kind(target) == KindSpecial {
		return Win{}
	}
	targetIsWild := l.Symbols.IsWild(target)
	n, m := 0, 1
	for _, p := range cells {
		s := g.At(p)
		if s != target && (targetIsWild || !l.Symbols.IsWild(s)) {
			break
		}
		n++
		m *= mult.at(p)
	}
	pay := l.Pays.Pay(target, n)
	if n < l.Pays.MinCount(target) || pay == 0 {
		return Win{}
	}
	return Win{
		Symbol:     target,
		Positions:  append([]Pos(nil), cells[:n]...),
		Pay:        pay * Units(m),
		Count:      n,
		Multiplier: m,
	}
}
