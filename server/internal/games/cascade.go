package games

import "pantheon-spins/server/internal/api"

// Tumble clears removed cells, lets the remaining symbols in each reel fall to
// the bottom keeping their order, and fills the gaps at the top with fill.
// It returns a new grid; g is not modified.
func Tumble(g Grid, removed []Pos, fill func(reel, row int) Symbol) Grid {
	out := g.Clone()
	gone := make([][]bool, len(g))
	for r := range g {
		gone[r] = make([]bool, len(g[r]))
	}
	for _, p := range removed {
		gone[p.Reel][p.Row] = true
	}
	for r, reel := range g {
		// Walk bottom-up, packing survivors at the bottom.
		dst := len(reel) - 1
		for row := len(reel) - 1; row >= 0; row-- {
			if !gone[r][row] {
				out[r][dst] = reel[row]
				dst--
			}
		}
		for row := dst; row >= 0; row-- {
			out[r][row] = fill(r, row)
		}
	}
	return out
}

// CascadeStep is one evaluation of the board during a cascade sequence.
type CascadeStep struct {
	Grid Grid
	// Events to animate before this step's wins (wild spread, multipliers…).
	Events []Event
	Wins   []Win
	// Removed cells are cleared after the wins; empty on the last step.
	Removed    []Pos
	Multiplier int
	Pay        Units
}

// Event is a game event in engine terms; ToAPI converts it.
type Event struct {
	Type      string
	Positions []Pos
	Symbol    Symbol
	HasSymbol bool
	Value     int64
	Data      map[string]any
}

// Evaluator finds the wins on a cascade step. It may mutate g (e.g. to convert
// symbols before evaluation) and report that as events. It returns the cells
// to clear, which are normally the winning cells; returning none ends the
// sequence.
type Evaluator func(step int, g Grid) (events []Event, wins []Win, removed []Pos, multiplier int)

// MaxCascadeSteps bounds a cascade sequence. Real sequences end long before;
// the cap only protects against a broken refill that keeps producing wins.
const MaxCascadeSteps = 200

// RunCascade evaluates start, then repeatedly tumbles away removed cells and
// re-evaluates until a step removes nothing. Wins are multiplied by the
// step's multiplier.
func RunCascade(start Grid, eval Evaluator, fill func(reel, row int) Symbol) []CascadeStep {
	var steps []CascadeStep
	g := start.Clone()
	for i := 0; i < MaxCascadeSteps; i++ {
		events, wins, removed, mult := eval(i, g)
		if mult < 1 {
			mult = 1
		}
		MultiplyWins(wins, mult)
		steps = append(steps, CascadeStep{
			Grid:       g.Clone(),
			Events:     events,
			Wins:       wins,
			Removed:    removed,
			Multiplier: mult,
			Pay:        TotalPay(wins),
		})
		if len(removed) == 0 {
			break
		}
		g = Tumble(g, removed, fill)
	}
	return steps
}

// CascadePay sums the pays of all steps.
func CascadePay(steps []CascadeStep) Units {
	var t Units
	for _, s := range steps {
		t += s.Pay
	}
	return t
}

// StepsAPI converts cascade steps to API steps at the given bet.
func (s *Symbols) StepsAPI(bet int64, steps []CascadeStep) []api.SpinStep {
	out := make([]api.SpinStep, len(steps))
	for i, st := range steps {
		m := st.Multiplier
		if m <= 1 {
			m = 0
		}
		out[i] = api.SpinStep{
			Grid:       s.GridAPI(st.Grid),
			Events:     s.EventsAPI(st.Events),
			Wins:       s.WinsAPI(bet, st.Wins),
			Removed:    PositionsAPI(st.Removed),
			Multiplier: m,
			StepWin:    st.Pay.Coins(bet),
		}
		if len(st.Removed) == 0 {
			out[i].Removed = nil
		}
	}
	return out
}

// EventsAPI converts events for the API.
func (s *Symbols) EventsAPI(events []Event) []api.GameEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]api.GameEvent, len(events))
	for i, e := range events {
		out[i] = api.GameEvent{Type: e.Type, Value: e.Value, Data: e.Data}
		if len(e.Positions) > 0 {
			out[i].Positions = PositionsAPI(e.Positions)
		}
		if e.HasSymbol {
			out[i].Symbol = s.ID(e.Symbol)
		}
	}
	return out
}
