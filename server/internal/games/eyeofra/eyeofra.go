// Package eyeofra implements Eye of Ra: a 6×5 cluster-pays slot with
// cascades whose multiplier grows with each cascade, and "Tomb Explorer", a
// pick-and-reveal bonus across three burial chambers.
//
// Base game: clusters of 5+ touching symbols pay and vanish; symbols above
// fall and new ones drop in. The multiplier steps up with every cascade and
// resets on the next spin. Four or more Eye of Ra scatters on the final board
// trigger Tomb Explorer.
//
// Tomb Explorer: each chamber is a row of sealed urns holding treasure, the
// passage to the next chamber, or (from chamber 2) a curse. Treasure is paid
// at the chamber's multiplier. The passage moves on; a curse ends the bonus
// but keeps everything collected. The last chamber holds the Pharaoh's
// treasure. Contents are fixed on the server when the bonus starts, so the
// order of picks cannot change the expected payout: clicking is presentation.
package eyeofra

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
)

const ID = "eye-of-ra"

// Event types sent to the client.
const (
	EventMaxWin = "max_win"
)

// Urn contents.
const (
	Treasure = "treasure"
	Passage  = "passage"
	Curse    = "curse"
	Pharaoh  = "pharaoh"
	Hidden   = "hidden"
)

type weighted struct {
	Value  float64 `json:"value"` // bet multiple for treasures
	Weight int     `json:"weight"`
}

type chamberParams struct {
	Urns       int        `json:"urns"`
	Curses     int        `json:"curses"`
	Passage    bool       `json:"passage"`
	Multiplier int        `json:"multiplier"`
	Treasures  []weighted `json:"treasures"`
	// PharaohX is the Pharaoh's treasure (bet multiple), or 0 for none.
	PharaohX float64 `json:"pharaohX"`
}

type params struct {
	Scatter         string          `json:"scatter"`
	TriggerCount    int             `json:"triggerCount"`
	MultiplierStep  int             `json:"multiplierStep"`
	MaxMultiplier   int             `json:"maxMultiplier"`
	MaxWinX         int64           `json:"maxWinX"`
	Chambers        []chamberParams `json:"chambers"`
	ExtraScatterMul int             `json:"extraScatterMultiplier"` // added to all chamber multipliers per scatter beyond the trigger
}

type chamber struct {
	urns       int
	curses     int
	passage    bool
	multiplier int
	values     []games.Units
	weights    []int
	pharaoh    games.Units
}

// Game is Eye of Ra.
type Game struct {
	*games.Game
	p        params
	scatter  games.Symbol
	base     games.ReelSet
	clusters games.Clusters
	chambers []chamber
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
	g := &Game{Game: cfg}
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
	g.scatter, err = cfg.Symbol(g.p.Scatter)
	check(err, "scatter")
	if err == nil && !cfg.Symbols.IsScatter(g.scatter) {
		check(errors.New("must be a scatter"), "scatter")
	}
	g.base, err = cfg.ReelSet("base")
	check(err, "reel set")
	g.clusters = games.Clusters{Symbols: cfg.Symbols, Pays: cfg.Pays, Topology: games.Square{}}
	if g.p.TriggerCount < 1 || g.p.MultiplierStep < 0 || g.p.MaxMultiplier < 1 || g.p.MaxWinX < 1 || g.p.ExtraScatterMul < 0 {
		check(errors.New("triggerCount, maxMultiplier and maxWinX must be positive; multiplierStep and extraScatterMultiplier non-negative"), "params")
	}
	if len(g.p.Chambers) == 0 {
		check(errors.New("at least one chamber"), "chambers")
	}
	for i, cp := range g.p.Chambers {
		last := i == len(g.p.Chambers)-1
		c := chamber{urns: cp.Urns, curses: cp.Curses, passage: cp.Passage, multiplier: cp.Multiplier}
		if cp.Passage == last {
			check(fmt.Errorf("chamber %d: every chamber but the last needs a passage", i+1), "chambers")
		}
		special := cp.Curses
		if cp.Passage {
			special++
		}
		if cp.PharaohX > 0 {
			special++
			if c.pharaoh, err = games.UnitsFromMultiple(cp.PharaohX); err != nil {
				check(err, fmt.Sprintf("chamber %d pharaohX", i+1))
			}
		}
		if cp.Urns < special+1 || cp.Urns > 25 || cp.Multiplier < 1 || cp.Curses < 0 {
			check(fmt.Errorf("chamber %d: needs 1–25 urns with room for treasure, multiplier ≥ 1", i+1), "chambers")
		}
		for _, t := range cp.Treasures {
			u, err := games.UnitsFromMultiple(t.Value)
			if err != nil || u == 0 || t.Weight < 0 {
				check(fmt.Errorf("chamber %d: bad treasure %+v", i+1, t), "chambers")
				continue
			}
			c.values = append(c.values, u)
			c.weights = append(c.weights, t.Weight)
		}
		if len(c.values) == 0 {
			check(fmt.Errorf("chamber %d: no treasures", i+1), "chambers")
		}
		g.chambers = append(g.chambers, c)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return g, nil
}

func (g *Game) isScatter(s games.Symbol) bool { return s == g.scatter }

// multiplierFor returns the cascade multiplier on step i (0-based).
func (g *Game) multiplierFor(step int) int {
	return min(1+step*g.p.MultiplierStep, g.p.MaxMultiplier)
}

// Spin plays one base-game spin with cascades.
func (g *Game) Spin(rng games.RNG, bet int64) (games.SpinOutcome, error) {
	return g.spinGrid(rng, g.base.Fill(rng, g.Heights), bet), nil
}

func (g *Game) spinGrid(rng games.RNG, start games.Grid, bet int64) games.SpinOutcome {
	eval := func(step int, grid games.Grid) ([]games.Event, []games.Win, []games.Pos, int) {
		wins := g.clusters.Evaluate(grid, nil)
		return nil, wins, games.WinPositions(wins), g.multiplierFor(step)
	}
	fill := func(reel, _ int) games.Symbol { return g.base[reel].Draw(rng) }
	steps := games.RunCascade(start, eval, fill)

	pay := games.CascadePay(steps)
	var after []games.Event
	if limit := games.Units(g.p.MaxWinX * games.UnitsPerBet); pay > limit {
		pay = limit
		after = append(after, games.Event{Type: EventMaxWin, Value: g.p.MaxWinX})
	}
	out := games.SpinOutcome{Outcome: api.SpinOutcome{
		Steps:        g.Symbols.StepsAPI(bet, steps),
		Events:       g.Symbols.EventsAPI(after),
		TotalWin:     pay.Coins(bet),
		Anticipation: games.Anticipation(start, g.isScatter, g.p.TriggerCount),
	}}

	final := steps[len(steps)-1].Grid
	scatters := final.Find(g.isScatter)
	if len(scatters) >= g.p.TriggerCount {
		extra := (len(scatters) - g.p.TriggerCount) * g.p.ExtraScatterMul
		out.Trigger = &api.BonusTrigger{
			Kind:      api.Pick,
			Positions: games.PositionsAPI(scatters),
			Data:      map[string]any{"scatters": len(scatters), "bonusMultiplier": extra},
		}
	}
	return out
}

// ---------- Tomb Explorer ----------

type urn struct {
	Kind     string      `json:"kind"`
	Value    games.Units `json:"value,omitempty"` // before the chamber multiplier
	Revealed bool        `json:"revealed,omitempty"`
}

type state struct {
	Bet     int64       `json:"bet"`
	Chamber int         `json:"chamber"` // 0-based
	Extra   int         `json:"extra"`   // added to every chamber multiplier
	Urns    [][]urn     `json:"urns"`    // all chambers, laid out at the start
	Win     games.Units `json:"win"`
	Done    bool        `json:"done"`
	Ending  string      `json:"ending,omitempty"` // curse, cleared or max_win
}

func (g *Game) chamberMultiplier(s state, c int) int { return g.chambers[c].multiplier + s.Extra }

// StartBonus seals every chamber's urns.
func (g *Game) StartBonus(rng games.RNG, bet int64, trigger api.BonusTrigger) (games.BonusState, error) {
	extra := 0
	if v, ok := trigger.Data["bonusMultiplier"]; ok {
		n, err := toInt(v)
		if err != nil || n < 0 {
			return games.BonusState{}, fmt.Errorf("%s: bad trigger data %v", ID, trigger.Data)
		}
		extra = n
	}
	s := state{Bet: bet, Extra: extra}
	for _, c := range g.chambers {
		urns := make([]urn, 0, c.urns)
		for range c.curses {
			urns = append(urns, urn{Kind: Curse})
		}
		if c.passage {
			urns = append(urns, urn{Kind: Passage})
		}
		if c.pharaoh > 0 {
			urns = append(urns, urn{Kind: Pharaoh, Value: c.pharaoh})
		}
		for len(urns) < c.urns {
			urns = append(urns, urn{Kind: Treasure, Value: c.values[games.Weighted(rng, c.weights)]})
		}
		// Fisher–Yates shuffle: positions are random, so pick order cannot matter.
		for i := len(urns) - 1; i > 0; i-- {
			j := rng.IntN(i + 1)
			urns[i], urns[j] = urns[j], urns[i]
		}
		s.Urns = append(s.Urns, urns)
	}
	return g.bonusState(s)
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		return int(n), nil
	}
	return 0, fmt.Errorf("not a number: %v", v)
}

// publicUrns shows revealed urns, plus everything once the bonus is over.
func (g *Game) publicUrns(s state) []any {
	out := make([]any, len(s.Urns))
	for c, urns := range s.Urns {
		row := make([]map[string]any, len(urns))
		for i, u := range urns {
			if !u.Revealed && !s.Done {
				row[i] = map[string]any{"kind": Hidden}
				continue
			}
			m := map[string]any{"kind": u.Kind, "revealed": u.Revealed}
			if u.Value > 0 {
				m["value"] = (u.Value * games.Units(g.chamberMultiplier(s, c))).Coins(s.Bet)
			}
			row[i] = m
		}
		out[c] = row
	}
	return out
}

func (g *Game) bonusState(s state) (games.BonusState, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return games.BonusState{}, err
	}
	mults := make([]int, len(g.chambers))
	for c := range g.chambers {
		mults[c] = g.chamberMultiplier(s, c)
	}
	bs := games.BonusState{
		Kind: api.Pick,
		Done: s.Done,
		Public: map[string]any{
			"chamber":     s.Chamber,
			"chambers":    len(g.chambers),
			"multipliers": mults,
			"urns":        g.publicUrns(s),
			"ending":      s.Ending,
		},
		Private: raw,
	}
	if !s.Done {
		var choices []string
		for i, u := range s.Urns[s.Chamber] {
			if !u.Revealed {
				choices = append(choices, strconv.Itoa(i))
			}
		}
		bs.Actions = []api.BonusActionOption{{Action: "pick", Choices: choices}}
	}
	return bs, nil
}

// BonusAction opens one urn in the current chamber.
func (g *Game) BonusAction(_ games.RNG, bs games.BonusState, a games.Action) (games.BonusState, games.BonusStep, error) {
	var s state
	if err := json.Unmarshal(bs.Private, &s); err != nil {
		return bs, games.BonusStep{}, fmt.Errorf("%s: decode state: %w", ID, err)
	}
	if s.Done || a.Name != "pick" {
		return bs, games.BonusStep{}, games.ErrInvalidAction
	}
	i, err := strconv.Atoi(a.Choice)
	urns := s.Urns[s.Chamber]
	if err != nil || i < 0 || i >= len(urns) || urns[i].Revealed {
		return bs, games.BonusStep{}, games.ErrInvalidAction
	}
	s, reveal := g.open(s, i)
	next, err := g.bonusState(s)
	if err != nil {
		return bs, games.BonusStep{}, err
	}
	win, _ := reveal["win"].(int64)
	return next, games.BonusStep{Win: win, Reveal: reveal}, nil
}

// open reveals urn i of the current chamber and applies it.
func (g *Game) open(s state, i int) (state, map[string]any) {
	c := s.Chamber
	u := &s.Urns[c][i]
	u.Revealed = true
	mult := g.chamberMultiplier(s, c)
	reveal := map[string]any{"chamber": c, "urn": i, "kind": u.Kind, "multiplier": mult}

	var pay games.Units
	switch u.Kind {
	case Treasure, Pharaoh:
		pay = u.Value * games.Units(mult)
	case Passage:
		s.Chamber++
		reveal["nextChamber"] = s.Chamber
	case Curse:
		s.Done = true
		s.Ending = Curse
	}
	if limit := games.Units(g.p.MaxWinX * games.UnitsPerBet); s.Win+pay >= limit {
		pay = limit - s.Win
		s.Done = true
		s.Ending = EventMaxWin
	}
	s.Win += pay
	// The last chamber is cleared once only curses remain sealed.
	if !s.Done && s.Chamber == len(s.Urns)-1 && !slices.ContainsFunc(s.Urns[s.Chamber], func(u urn) bool { return !u.Revealed && u.Kind != Curse }) {
		s.Done = true
		s.Ending = "cleared"
	}
	reveal["win"] = pay.Coins(s.Bet)
	if s.Done {
		reveal["ending"] = s.Ending
	}
	return s, reveal
}
