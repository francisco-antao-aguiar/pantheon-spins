package valhalla

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
)

const configDir = "../../../../game-configs"

func load(t testing.TB) *Game {
	t.Helper()
	sg, err := New(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return sg.(*Game)
}

// Short names for drawing grids.
var alias = map[string]string{
	"f": "fehu", "u": "uruz", "t": "thurisaz", "a": "ansuz",
	"S": "shield", "X": "axe", "H": "horn", "V": "valkyrie",
	"W": "wild", "O": "bonus_odin", "T": "bonus_thor", "L": "bonus_loki",
}

// grid draws a 5×4 grid from four rows of five characters.
func grid(g *Game, rows ...string) games.Grid {
	out := games.NewGrid(g.Heights)
	for row, line := range rows {
		cells := strings.Fields(line)
		for reel, c := range cells {
			out[reel][row] = g.Symbols.MustLookup(alias[c])
		}
	}
	return out
}

// noWin is a board with no win: reels 1 and 2 share no symbol.
var noWin = []string{
	"f u f u f",
	"t a t a t",
	"S X S X S",
	"H V H V H",
}

func TestConfigIsValid(t *testing.T) {
	g := load(t)
	info := g.Info()
	if info.Id != ID || info.Reels != 5 || info.Rows != 4 || info.Layout != api.Ways {
		t.Fatalf("info: %+v", info)
	}
	if lo, hi := g.TargetRange(); lo != 0.94 || hi != 0.97 {
		t.Fatalf("target %v–%v", lo, hi)
	}
}

func TestBaseSpin(t *testing.T) {
	g := load(t)
	rng := &gamestest.ScriptedRNG{}
	tests := []struct {
		name      string
		rows      []string
		wantWin   int64 // at bet 100: 1 unit = 5 Coins
		wantGod   God
		wantSpins int
		wantAnt   []int
	}{
		{name: "nothing", rows: noWin},
		{
			name:    "valkyrie on three reels, one way",
			rows:    []string{"V V V u f", "t a t a t", "S X S X S", "H f H f H"},
			wantWin: 40, // 0.4× of 100
		},
		{
			name:    "wild substitutes: 2 × 2 ways of valkyrie on four reels",
			rows:    []string{"V V V V f", "V W t S t", "f S a X S", "u X H H X"},
			wantWin: 4 * 100, // 1× per way, 4 ways
		},
		{
			name:    "two bonus symbols: anticipation, no trigger",
			rows:    []string{"O u T u f", "t a t a t", "S X S X S", "H f H f H"},
			wantAnt: []int{3, 4},
		},
		{
			name: "three Odin", rows: []string{"O u O u O", "t a t a t", "S X S X S", "H f H f H"},
			wantGod: Odin, wantSpins: 10, wantAnt: []int{3, 4},
		},
		{
			name: "majority wins: two Thor, one Loki", rows: []string{"T u L u f", "t a t T t", "S X S X S", "H f H f H"},
			wantGod: Thor, wantSpins: 10, wantAnt: []int{3, 4},
		},
		{
			name: "four symbols award 12 spins", rows: []string{"L u L u L", "t L t a t", "S X S X S", "H f H f H"},
			wantGod: Loki, wantSpins: 12, wantAnt: []int{2, 3, 4},
		},
		{
			name: "six symbols award 15 spins", rows: []string{"L L L u f", "O O O a t", "S X S X S", "H f H f H"},
			wantGod: Odin, wantSpins: 15, wantAnt: []int{1, 2, 3, 4}, // 3–3 tie broken by the scripted RNG (index 0 = Odin)
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := g.spinGrid(rng, grid(g, tc.rows...), 100)
			if out.Outcome.TotalWin != tc.wantWin || out.Outcome.Steps[0].StepWin != tc.wantWin {
				t.Fatalf("win %d, want %d (wins %+v)", out.Outcome.TotalWin, tc.wantWin, out.Outcome.Steps[0].Wins)
			}
			if !slices.Equal(out.Outcome.Anticipation, tc.wantAnt) {
				t.Fatalf("anticipation %v, want %v", out.Outcome.Anticipation, tc.wantAnt)
			}
			if tc.wantGod == "" {
				if out.Trigger != nil {
					t.Fatalf("unexpected trigger %+v", out.Trigger)
				}
				return
			}
			if out.Trigger == nil {
				t.Fatal("no trigger")
			}
			if out.Trigger.Data["god"] != string(tc.wantGod) || out.Trigger.Data["spins"] != tc.wantSpins {
				t.Fatalf("trigger data %v, want %s / %d", out.Trigger.Data, tc.wantGod, tc.wantSpins)
			}
		})
	}
}

func TestTiesAreBrokenAtRandom(t *testing.T) {
	g := load(t)
	votes := map[God]int{Odin: 1, Thor: 1, Loki: 1}
	for i, want := range []God{Odin, Thor, Loki} {
		if got := g.chooseGod(&gamestest.ScriptedRNG{Values: []int{i}}, votes); got != want {
			t.Fatalf("draw %d chose %s, want %s", i, got, want)
		}
	}
	if got := g.chooseGod(&gamestest.ScriptedRNG{Values: []int{2}}, map[God]int{Thor: 2, Loki: 1}); got != Thor {
		t.Fatalf("majority ignored: %s", got)
	}
}

func startState(t *testing.T, g *Game, god God, spins any) games.BonusState {
	t.Helper()
	bs, err := g.StartBonus(nil, 100, api.BonusTrigger{Kind: api.FreeSpins, Data: map[string]any{"god": string(god), "spins": spins}})
	if err != nil {
		t.Fatal(err)
	}
	return bs
}

func TestBonusStateMachine(t *testing.T) {
	g := load(t)
	// Spins may arrive as float64 when trigger data went through JSON.
	bs := startState(t, g, Thor, float64(10))
	if bs.Done || bs.Kind != api.FreeSpins || len(bs.Actions) != 1 || bs.Actions[0].Action != "spin" {
		t.Fatalf("start: %+v", bs)
	}
	if bs.Public["god"] != "thor" || bs.Public["spinsLeft"] != 10 || bs.Public["totalSpins"] != 10 {
		t.Fatalf("public state: %v", bs.Public)
	}
	if _, err := json.Marshal(bs.Public); err != nil {
		t.Fatal(err)
	}

	if _, _, err := g.BonusAction(games.NewCryptoRNG(), bs, games.Action{Name: "pick"}); !errors.Is(err, games.ErrInvalidAction) {
		t.Fatalf("wrong action: %v", err)
	}

	rng := games.NewCryptoRNG()
	played, total := 0, int64(0)
	for !bs.Done {
		next, step, err := g.BonusAction(rng, bs, games.Action{Name: "spin"})
		if err != nil {
			t.Fatal(err)
		}
		if step.Outcome == nil || len(step.Outcome.Steps) != 1 || step.Win != step.Outcome.TotalWin {
			t.Fatalf("step %d: %+v", played, step)
		}
		played++
		total += step.Win
		bs = next
		if played > 50 {
			t.Fatal("bonus exceeded maxFreeSpins")
		}
	}
	if bs.Public["spinsLeft"] != 0 || bs.Public["spinsPlayed"] != played || len(bs.Actions) != 0 {
		t.Fatalf("end state: %+v", bs)
	}
	var s state
	_ = json.Unmarshal(bs.Private, &s)
	if s.Win.Coins(100) != total {
		t.Fatalf("private win %d, sum of steps %d", s.Win.Coins(100), total)
	}
	if _, _, err := g.BonusAction(rng, bs, games.Action{Name: "spin"}); !errors.Is(err, games.ErrInvalidAction) {
		t.Fatalf("action after the end: %v", err)
	}

	for _, bad := range []map[string]any{{"god": "freya", "spins": 10}, {"god": "odin"}, {"god": "odin", "spins": 0}} {
		if _, err := g.StartBonus(nil, 100, api.BonusTrigger{Data: bad}); err == nil {
			t.Fatalf("accepted bad trigger %v", bad)
		}
	}
}

func TestOdinRavens(t *testing.T) {
	g := load(t)
	rng := games.NewCryptoRNG()
	landed := grid(g, "O u f u f", "t a t a t", "S X S X S", "H f H f H")
	seen := 0
	for range 300 {
		_, step := g.freeSpin(rng, state{God: Odin, Bet: 100, SpinsLeft: 5, Total: 5}, landed)
		ev := step.Outcome.Steps[0].Events
		if len(ev) == 0 {
			continue
		}
		seen++
		e := ev[0]
		if e.Type != EventRavenWilds || e.Symbol != "wild" || len(e.Positions) < 1 || len(e.Positions) > 5 {
			t.Fatalf("event %+v", e)
		}
		for _, p := range e.Positions {
			if p.Reel == 0 {
				t.Fatalf("raven on reel 1: %+v", p)
			}
		}
		// The step shows the landed grid; events change it before evaluation.
		if step.Outcome.Steps[0].Grid[0][0] != "bonus_odin" {
			t.Fatal("step grid is not the landed grid")
		}
	}
	if seen == 0 {
		t.Fatal("ravens never appeared")
	}
}

func TestThorLightning(t *testing.T) {
	g := load(t)
	rng := games.NewCryptoRNG()
	// Valkyrie on three reels pays 0.4× = 8 units without a multiplier.
	landed := grid(g, "V V V u f", "t a t a t", "S X S X S", "H f H f H")
	allowed := map[int64]bool{}
	for _, v := range g.thor.values {
		allowed[int64(v)] = v > 1
	}
	seen := 0
	for range 300 {
		_, step := g.freeSpin(rng, state{God: Thor, Bet: 100, SpinsLeft: 5, Total: 5}, landed)
		st := step.Outcome.Steps[0]
		m := int64(1)
		if len(st.Events) > 0 {
			e := st.Events[0]
			if e.Type != EventLightning || !allowed[e.Value] {
				t.Fatalf("event %+v", e)
			}
			m = e.Value
			seen++
		}
		if step.Win != 40*m || int64(st.Multiplier) != map[bool]int64{true: m, false: 0}[m > 1] {
			t.Fatalf("win %d with multiplier %d (step multiplier %d)", step.Win, m, st.Multiplier)
		}
	}
	if seen == 0 {
		t.Fatal("lightning never struck")
	}
}

func TestLokiTransform(t *testing.T) {
	g := load(t)
	rng := games.NewCryptoRNG()
	landed := grid(g, noWin...)
	seen := 0
	for range 300 {
		_, step := g.freeSpin(rng, state{God: Loki, Bet: 100, SpinsLeft: 5, Total: 5}, landed)
		ev := step.Outcome.Steps[0].Events
		if len(ev) == 0 {
			continue
		}
		seen++
		e := ev[0]
		sym, ok := g.Symbols.Lookup(e.Symbol)
		if e.Type != EventTransform || !ok || !g.Symbols.IsPaying(sym) {
			t.Fatalf("event %+v", e)
		}
		if n := len(e.Positions); !slices.Contains(g.lokiCells.values, n) {
			t.Fatalf("transformed %d cells", n)
		}
		// Wins must come from the transformed grid.
		after := landed.Clone()
		for _, p := range e.Positions {
			after[p.Reel][p.Row] = sym
		}
		want := games.TotalPay(g.ways.Evaluate(after, nil)).Coins(100)
		if step.Win != want {
			t.Fatalf("win %d, want %d from the transformed grid", step.Win, want)
		}
	}
	if seen == 0 {
		t.Fatal("Loki never transformed")
	}
}

func TestRetrigger(t *testing.T) {
	g := load(t)
	rng := &gamestest.ScriptedRNG{} // Thor multiplier draw → 1, so no lightning
	landed := grid(g, "O u T u L", "t a t a t", "S X S X S", "H f H f H")

	s, step := g.freeSpin(rng, state{God: Thor, Bet: 100, SpinsLeft: 3, Total: 10}, landed)
	if s.SpinsLeft != 2+5 || s.Total != 15 {
		t.Fatalf("after retrigger: left %d total %d", s.SpinsLeft, s.Total)
	}
	if ev := step.Outcome.Events; len(ev) != 1 || ev[0].Type != EventRetrigger || ev[0].Value != 5 {
		t.Fatalf("events %+v", ev)
	}

	// The total is capped at maxFreeSpins (50).
	s, step = g.freeSpin(rng, state{God: Thor, Bet: 100, SpinsLeft: 3, Total: 48}, landed)
	if s.Total != 50 || step.Outcome.Events[0].Value != 2 {
		t.Fatalf("capped retrigger: total %d events %+v", s.Total, step.Outcome.Events)
	}
	s, step = g.freeSpin(rng, state{God: Thor, Bet: 100, SpinsLeft: 3, Total: 50}, landed)
	if s.Total != 50 || len(step.Outcome.Events) != 0 {
		t.Fatalf("retrigger beyond the cap: total %d events %+v", s.Total, step.Outcome.Events)
	}
}

func TestWinCap(t *testing.T) {
	g := load(t)
	landed := grid(g, "V V V u f", "t a t a t", "S X S X S", "H f H f H") // pays 8 units
	limit := games.Units(g.p.MaxWinX * games.UnitsPerBet)
	s, step := g.freeSpin(&gamestest.ScriptedRNG{}, state{God: Thor, Bet: 100, SpinsLeft: 5, Total: 5, Win: limit - 3}, landed)
	if !s.Capped || s.Win != limit || step.Win != games.Units(3).Coins(100) {
		t.Fatalf("capped=%v win=%d step=%d", s.Capped, s.Win, step.Win)
	}
	if ev := step.Outcome.Events; len(ev) == 0 || ev[0].Type != EventMaxWin {
		t.Fatalf("events %+v", ev)
	}
	bs, _ := g.bonusState(s)
	if !bs.Done {
		t.Fatal("bonus continues after the win cap")
	}
}

// TestGodBalance reports each god's average bonus. They need not be equal
// (the symbols decide, not the player), but should be in the same range.
func TestGodBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	g := load(t)
	rng := games.NewCryptoRNG()
	const n = 20_000
	for _, god := range gods {
		var total int64
		for range n {
			bs := startState(t, g, god, 10)
			for !bs.Done {
				next, step, err := g.BonusAction(rng, bs, games.Action{Name: "spin"})
				if err != nil {
					t.Fatal(err)
				}
				total += step.Win
				bs = next
			}
		}
		avg := float64(total) / n / 100
		t.Logf("%-4s 10 free spins average %.1f× bet", god, avg)
		if avg < 20 || avg > 150 {
			t.Errorf("%s average %.1f× is out of the expected range", god, avg)
		}
	}
}
