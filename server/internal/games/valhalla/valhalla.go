// Package valhalla implements Halls of Valhalla: a 5×4, 1024-ways slot whose
// "Ragnarök Free Spins" are led by the god the bonus symbols choose.
//
// Three or more bonus symbols (Odin, Thor or Loki) trigger the free spins.
// The god with the most symbols leads them (ties are broken at random):
//
//   - Odin: ravens turn random cells on reels 2–5 into wilds.
//   - Thor: lightning strikes multiply the spin's win.
//   - Loki: a group of cells transforms into one random symbol.
//
// The player never chooses: the landed symbols decide.
package valhalla

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
)

const ID = "halls-of-valhalla"

type God string

const (
	Odin God = "odin"
	Thor God = "thor"
	Loki God = "loki"
)

var gods = []God{Odin, Thor, Loki}

// Event types sent to the client.
const (
	EventRavenWilds = "raven_wilds"    // positions become wild
	EventLightning  = "lightning"      // value is the spin multiplier
	EventTransform  = "loki_transform" // positions become symbol
	EventRetrigger  = "retrigger"      // value is the extra free spins
	EventMaxWin     = "max_win"        // the bonus hit its win cap
)

// params is the game-specific part of the config.
type params struct {
	Wild      string         `json:"wild"`
	Scatters  map[God]string `json:"scatters"`
	FreeSpins map[string]int `json:"freeSpins"` // bonus-symbol count → spins
	Retrigger struct {
		Scatters int `json:"scatters"`
		Spins    int `json:"spins"`
	} `json:"retrigger"`
	MaxFreeSpins int   `json:"maxFreeSpins"`
	MaxWinX      int64 `json:"maxWinX"`
	Odin         struct {
		Ravens []weighted `json:"ravens"`
		Reels  []int      `json:"reels"`
	} `json:"odin"`
	Thor struct {
		Multipliers []weighted `json:"multipliers"`
	} `json:"thor"`
	Loki struct {
		Cells   []weighted     `json:"cells"`
		Targets map[string]int `json:"targets"`
	} `json:"loki"`
}

type weighted struct {
	Value  int `json:"value"`
	Weight int `json:"weight"`
}

// table draws a value with fixed weights.
type table struct {
	values, weights []int
}

func newTable(ws []weighted, minValue int) (table, error) {
	var t table
	for _, w := range ws {
		if w.Value < minValue || w.Weight < 0 {
			return t, fmt.Errorf("value %d / weight %d out of range", w.Value, w.Weight)
		}
		t.values = append(t.values, w.Value)
		t.weights = append(t.weights, w.Weight)
	}
	total := 0
	for _, w := range t.weights {
		total += w
	}
	if total == 0 {
		return t, errors.New("weights sum to zero")
	}
	return t, nil
}

func (t table) draw(rng games.RNG) int { return t.values[games.Weighted(rng, t.weights)] }

// Game is Halls of Valhalla.
type Game struct {
	*games.Game
	p           params
	wild        games.Symbol
	scatterGod  map[games.Symbol]God
	base, free  games.ReelSet
	ways        games.Ways
	spinsFor    []int // index: bonus-symbol count
	ravens      table
	ravenReels  []int
	thor        table
	lokiCells   table
	lokiTargets []games.Symbol
	lokiWeights []int
}

var _ games.SlotGame = (*Game)(nil)

// New loads the game from its config.
func New(configDir string) (games.SlotGame, error) {
	cfg, err := games.LoadConfig(configDir, ID)
	if err != nil {
		return nil, err
	}
	return FromConfig(cfg)
}

// FromConfig builds the game from an already-loaded config.
func FromConfig(cfg *games.Game) (*Game, error) {
	g := &Game{Game: cfg, scatterGod: map[games.Symbol]God{}}
	if err := cfg.DecodeParams(&g.p); err != nil {
		return nil, err
	}
	var errs []error
	check := func(err error, what string) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", ID, what, err))
		}
	}

	var err error
	g.wild, err = cfg.Symbol(g.p.Wild)
	check(err, "wild")
	for _, god := range gods {
		s, err := cfg.Symbol(g.p.Scatters[god])
		check(err, "scatter for "+string(god))
		if err == nil {
			if !cfg.Symbols.IsScatter(s) {
				check(errors.New("must be a scatter"), "scatter for "+string(god))
			}
			g.scatterGod[s] = god
		}
	}
	g.base, err = cfg.ReelSet("base")
	check(err, "reel set")
	g.free, err = cfg.ReelSet("free")
	check(err, "reel set")
	g.ways = games.Ways{Symbols: cfg.Symbols, Pays: cfg.Pays}

	// Free spins by count, filled forward from the smallest key (3).
	maxCount := cfg.Reels * cfg.Rows
	g.spinsFor = make([]int, maxCount+1)
	for k, v := range g.p.FreeSpins {
		n, err := strconv.Atoi(k)
		if err != nil || n < 1 || n > maxCount || v < 1 {
			check(fmt.Errorf("bad entry %q: %d", k, v), "freeSpins")
			continue
		}
		g.spinsFor[n] = v
	}
	if g.spinsFor[3] == 0 {
		check(errors.New(`needs an entry for "3"`), "freeSpins")
	}
	for n := 4; n <= maxCount; n++ {
		if g.spinsFor[n] == 0 {
			g.spinsFor[n] = g.spinsFor[n-1]
		}
	}
	if g.p.Retrigger.Scatters < 1 || g.p.Retrigger.Spins < 0 {
		check(errors.New("scatters must be ≥ 1 and spins ≥ 0"), "retrigger")
	}
	if g.p.MaxFreeSpins < g.spinsFor[maxCount] {
		check(errors.New("must be at least the largest award"), "maxFreeSpins")
	}
	if g.p.MaxWinX < 1 {
		check(errors.New("must be positive"), "maxWinX")
	}

	g.ravens, err = newTable(g.p.Odin.Ravens, 0)
	check(err, "odin.ravens")
	g.ravenReels = g.p.Odin.Reels
	for _, r := range g.ravenReels {
		if r < 0 || r >= cfg.Reels {
			check(fmt.Errorf("reel %d", r), "odin.reels")
		}
	}
	g.thor, err = newTable(g.p.Thor.Multipliers, 1)
	check(err, "thor.multipliers")
	g.lokiCells, err = newTable(g.p.Loki.Cells, 0)
	check(err, "loki.cells")
	ids := make([]string, 0, len(g.p.Loki.Targets))
	for id := range g.p.Loki.Targets {
		ids = append(ids, id)
	}
	slices.Sort(ids) // deterministic order for scripted tests
	for _, id := range ids {
		s, err := cfg.Symbol(id)
		check(err, "loki.targets")
		if err == nil && !cfg.Symbols.IsPaying(s) {
			check(fmt.Errorf("%s is not a paying symbol", id), "loki.targets")
		}
		g.lokiTargets = append(g.lokiTargets, s)
		g.lokiWeights = append(g.lokiWeights, g.p.Loki.Targets[id])
	}
	if len(g.lokiTargets) == 0 {
		check(errors.New("empty"), "loki.targets")
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return g, nil
}

func (g *Game) isScatter(s games.Symbol) bool { _, ok := g.scatterGod[s]; return ok }

// Spin plays one base-game spin.
func (g *Game) Spin(rng games.RNG, bet int64) (games.SpinOutcome, error) {
	return g.spinGrid(rng, g.base.Fill(rng, g.Heights), bet), nil
}

// spinGrid evaluates a landed base-game grid.
func (g *Game) spinGrid(rng games.RNG, grid games.Grid, bet int64) games.SpinOutcome {
	wins := g.ways.Evaluate(grid, nil)
	step := games.CascadeStep{Grid: grid, Wins: wins, Pay: games.TotalPay(wins), Multiplier: 1}
	out := games.SpinOutcome{Outcome: api.SpinOutcome{
		Steps:        g.Symbols.StepsAPI(bet, []games.CascadeStep{step}),
		TotalWin:     step.Pay.Coins(bet),
		Anticipation: games.Anticipation(grid, g.isScatter, 3),
	}}

	scatters := grid.Find(g.isScatter)
	if len(scatters) < 3 {
		return out
	}
	votes := map[God]int{}
	for _, p := range scatters {
		votes[g.scatterGod[grid.At(p)]]++
	}
	god := g.chooseGod(rng, votes)
	out.Trigger = &api.BonusTrigger{
		Kind:         api.FreeSpins,
		Positions:    games.PositionsAPI(scatters),
		Anticipation: out.Outcome.Anticipation,
		Data: map[string]any{
			"god":   string(god),
			"spins": g.spinsFor[len(scatters)],
			"votes": map[string]any{"odin": votes[Odin], "thor": votes[Thor], "loki": votes[Loki]},
		},
	}
	return out
}

// chooseGod returns the god with the most votes, breaking ties at random.
func (g *Game) chooseGod(rng games.RNG, votes map[God]int) God {
	best := 0
	for _, n := range votes {
		best = max(best, n)
	}
	var tied []God
	for _, god := range gods {
		if votes[god] == best {
			tied = append(tied, god)
		}
	}
	return tied[rng.IntN(len(tied))]
}

// state is the private bonus state.
type state struct {
	God       God         `json:"god"`
	Bet       int64       `json:"bet"`
	SpinsLeft int         `json:"spinsLeft"`
	Played    int         `json:"played"`
	Total     int         `json:"total"`
	Win       games.Units `json:"win"`
	Capped    bool        `json:"capped"`
}

// StartBonus sets up the free spins the trigger awarded.
func (g *Game) StartBonus(_ games.RNG, bet int64, trigger api.BonusTrigger) (games.BonusState, error) {
	god := God(fmt.Sprint(trigger.Data["god"]))
	if !slices.Contains(gods, god) {
		return games.BonusState{}, fmt.Errorf("%s: trigger has no valid god: %v", ID, trigger.Data)
	}
	spins, err := toInt(trigger.Data["spins"])
	if err != nil || spins < 1 {
		return games.BonusState{}, fmt.Errorf("%s: trigger has no valid spins: %v", ID, trigger.Data)
	}
	return g.bonusState(state{God: god, Bet: bet, SpinsLeft: spins, Total: spins})
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	}
	return 0, fmt.Errorf("not a number: %v", v)
}

func (g *Game) bonusState(s state) (games.BonusState, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return games.BonusState{}, err
	}
	done := s.SpinsLeft == 0 || s.Capped
	bs := games.BonusState{
		Kind: api.FreeSpins,
		Done: done,
		Public: map[string]any{
			"god":         string(s.God),
			"spinsLeft":   s.SpinsLeft,
			"spinsPlayed": s.Played,
			"totalSpins":  s.Total,
		},
		Private: raw,
	}
	if !done {
		bs.Actions = []api.BonusActionOption{{Action: "spin"}}
	}
	return bs, nil
}

// BonusAction plays the next free spin.
func (g *Game) BonusAction(rng games.RNG, bs games.BonusState, a games.Action) (games.BonusState, games.BonusStep, error) {
	var s state
	if err := json.Unmarshal(bs.Private, &s); err != nil {
		return bs, games.BonusStep{}, fmt.Errorf("%s: decode state: %w", ID, err)
	}
	if bs.Done || a.Name != "spin" {
		return bs, games.BonusStep{}, games.ErrInvalidAction
	}
	s, step := g.freeSpin(rng, s, g.free.Fill(rng, g.Heights))
	next, err := g.bonusState(s)
	if err != nil {
		return bs, games.BonusStep{}, err
	}
	return next, step, nil
}

// freeSpin plays one free spin on a landed grid: the god's feature, the
// evaluation, the win cap and retriggers.
func (g *Game) freeSpin(rng games.RNG, s state, landed games.Grid) (state, games.BonusStep) {
	grid := landed.Clone()
	var events []games.Event
	mult := 1
	switch s.God {
	case Odin:
		if cells := g.pickCells(rng, grid, g.ravens.draw(rng), g.ravenReels); len(cells) > 0 {
			for _, p := range cells {
				grid.Set(p, g.wild)
			}
			events = append(events, games.Event{Type: EventRavenWilds, Positions: cells, Symbol: g.wild, HasSymbol: true})
		}
	case Thor:
		if mult = g.thor.draw(rng); mult > 1 {
			events = append(events, games.Event{Type: EventLightning, Value: int64(mult)})
		}
	case Loki:
		if n := g.lokiCells.draw(rng); n > 0 {
			target := g.lokiTargets[games.Weighted(rng, g.lokiWeights)]
			cells := g.pickCells(rng, grid, n, nil)
			for _, p := range cells {
				grid.Set(p, target)
			}
			events = append(events, games.Event{Type: EventTransform, Positions: cells, Symbol: target, HasSymbol: true})
		}
	}

	wins := g.ways.Evaluate(grid, nil)
	games.MultiplyWins(wins, mult)
	pay := games.TotalPay(wins)

	var after []games.Event
	// Win cap: the bonus pays at most MaxWinX × bet in total.
	if limit := games.Units(g.p.MaxWinX * games.UnitsPerBet); s.Win+pay >= limit {
		pay = limit - s.Win
		s.Capped = true
		after = append(after, games.Event{Type: EventMaxWin, Value: g.p.MaxWinX})
	}
	s.Win += pay
	s.SpinsLeft--
	s.Played++

	if !s.Capped && grid.Count(g.isScatter) >= g.p.Retrigger.Scatters {
		if extra := min(g.p.Retrigger.Spins, g.p.MaxFreeSpins-s.Total); extra > 0 {
			s.SpinsLeft += extra
			s.Total += extra
			after = append(after, games.Event{Type: EventRetrigger, Value: int64(extra)})
		}
	}

	step := games.CascadeStep{Grid: landed, Events: events, Wins: wins, Pay: pay, Multiplier: mult}
	out := api.SpinOutcome{
		Steps:    g.Symbols.StepsAPI(s.Bet, []games.CascadeStep{step}),
		Events:   g.Symbols.EventsAPI(after),
		TotalWin: pay.Coins(s.Bet),
	}
	return s, games.BonusStep{
		Win:     pay.Coins(s.Bet),
		Outcome: &out,
		Reveal:  map[string]any{"god": string(s.God), "multiplier": mult},
	}
}

// pickCells picks up to n distinct random cells that are neither wild nor a
// bonus symbol, restricted to the given reels (all reels when nil).
func (g *Game) pickCells(rng games.RNG, grid games.Grid, n int, reels []int) []games.Pos {
	if n <= 0 {
		return nil
	}
	var cand []games.Pos
	for r, reel := range grid {
		if reels != nil && !slices.Contains(reels, r) {
			continue
		}
		for row, s := range reel {
			if s != g.wild && !g.isScatter(s) {
				cand = append(cand, games.Pos{Reel: r, Row: row})
			}
		}
	}
	n = min(n, len(cand))
	// Partial Fisher–Yates shuffle.
	for i := range n {
		j := i + rng.IntN(len(cand)-i)
		cand[i], cand[j] = cand[j], cand[i]
	}
	out := cand[:n]
	slices.SortFunc(out, func(a, b games.Pos) int {
		if a.Reel != b.Reel {
			return a.Reel - b.Reel
		}
		return a.Row - b.Row
	})
	return out
}
