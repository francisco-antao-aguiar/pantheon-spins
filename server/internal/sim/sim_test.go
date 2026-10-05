package sim_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
	"pantheon-spins/server/internal/sim"
)

func TestFakeGameMatchesClosedForm(t *testing.T) {
	// Base: 3× bet with probability 1/4 → 0.75. Bonus: 1 in 50, three picks
	// of 5× each → 15×/50 = 0.30. Total RTP 1.05.
	game := &gamestest.Fake{ID: "fake", WinEvery: 4, WinMultiplier: 3, BonusEvery: 50, Picks: 3, PickValue: 5}
	rep, err := sim.Run(context.Background(), game, sim.Options{Spins: 2_000_000, Bet: 20})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Spins != 2_000_000 || rep.TotalBet != 40_000_000 {
		t.Fatalf("spins %d total bet %d", rep.Spins, rep.TotalBet)
	}
	near(t, "RTP", rep.RTP(), 1.05, 4*rep.RTPConfidence())
	near(t, "base RTP", rep.BaseRTP(), 0.75, 0.01)
	near(t, "bonus frequency", rep.BonusFrequency(), 0.02, 0.001)
	near(t, "hit frequency", rep.HitFrequency(), 1-0.75*0.98, 0.002)
	if rep.AvgBonusPayout() != 15 {
		t.Fatalf("avg bonus payout %v, want exactly 15", rep.AvgBonusPayout())
	}
	if rep.MaxWinX() != 18 {
		t.Fatalf("max win %v×, want 18× (3× base + 15× bonus)", rep.MaxWinX())
	}
	var total int64
	for _, c := range rep.Histogram {
		total += c
	}
	if total != rep.Spins {
		t.Fatalf("histogram covers %d spins, want %d", total, rep.Spins)
	}
}

// waysGame is a minimal ways game built from the sample config.
type waysGame struct {
	*games.Game
	base games.ReelSet
	eval games.Ways
}

func (w *waysGame) Spin(rng games.RNG, bet int64) (games.SpinOutcome, error) {
	g := w.base.Fill(rng, w.Heights)
	pay := games.TotalPay(w.eval.Evaluate(g, nil))
	return games.SpinOutcome{Outcome: api.SpinOutcome{TotalWin: pay.Coins(bet)}}, nil
}

func (w *waysGame) StartBonus(games.RNG, int64, api.BonusTrigger) (games.BonusState, error) {
	return games.BonusState{}, errors.New("no bonus")
}

func (w *waysGame) BonusAction(games.RNG, games.BonusState, games.Action) (games.BonusState, games.BonusStep, error) {
	return games.BonusState{}, games.BonusStep{}, errors.New("no bonus")
}

func TestWaysGameMatchesAnalyticRTP(t *testing.T) {
	cfg, err := games.LoadConfig("../games/testdata", "sample-ways")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := cfg.ReelSet("base")
	game := &waysGame{Game: cfg, base: base, eval: games.Ways{Symbols: cfg.Symbols, Pays: cfg.Pays}}

	want := analyticWaysRTP(cfg, base)
	rep, err := sim.Run(context.Background(), game, sim.Options{Spins: 2_000_000})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("analytic RTP %.4f%%, simulated %.4f%% ± %.4f%%", 100*want, 100*rep.RTP(), 100*rep.RTPConfidence())
	near(t, "RTP", rep.RTP(), want, 4*rep.RTPConfidence())
}

// analyticWaysRTP computes the exact RTP of a ways game whose cells are drawn
// independently: the expected number of ways for a chain of exactly L reels is
// the product of expected matches on reels 0..L-1 times P(no match on reel L).
func analyticWaysRTP(cfg *games.Game, base games.ReelSet) float64 {
	wild := cfg.Symbols.MustLookup("wild")
	var expected float64 // in units per spin
	for _, s := range cfg.Symbols.OfKind(games.KindLow, games.KindHigh) {
		chain := 1.0 // expected ways so far
		for r := 0; r < cfg.Reels; r++ {
			p := float64(base[r].Weight(s)+base[r].Weight(wild)) / float64(base[r].Total())
			rows := float64(cfg.Heights[r])
			pNone := math.Pow(1-p, rows)
			if r > 0 {
				// Chain of exactly r reels ends here.
				expected += float64(cfg.Pays.Pay(s, r)) * chain * pNone
			}
			chain *= rows * p
		}
		expected += float64(cfg.Pays.Pay(s, cfg.Reels)) * chain
	}
	return expected / games.UnitsPerBet
}

func TestNeverEndingBonusFails(t *testing.T) {
	game := &gamestest.Fake{ID: "stuck", BonusEvery: 1, Picks: 1_000_000, PickValue: 1}
	_, err := sim.Run(context.Background(), game, sim.Options{Spins: 10, Bet: 20, Workers: 2})
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("got %v, want a did-not-finish error", err)
	}
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	game := &gamestest.Fake{ID: "fake", WinEvery: 2, WinMultiplier: 1}
	if _, err := sim.Run(ctx, game, sim.Options{Spins: 1_000_000, Bet: 20}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestChooseAction(t *testing.T) {
	st := games.BonusState{Actions: []api.BonusActionOption{
		{Action: "spin"},
		{Action: "pick", Choices: []string{"a", "b", "c"}},
	}}
	rng := &gamestest.ScriptedRNG{Values: []int{1, 2}}
	a, ok := sim.ChooseAction(rng, st)
	if !ok || a.Name != "pick" || a.Choice != "c" {
		t.Fatalf("got %+v %v", a, ok)
	}
	if _, ok := sim.ChooseAction(rng, games.BonusState{}); ok {
		t.Fatal("chose an action when none is offered")
	}
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.5f, want %.5f ± %.5f", what, got, want, tol)
	}
}
