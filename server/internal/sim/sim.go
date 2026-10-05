// Package sim runs a slot game for millions of spins in parallel and reports
// its maths: RTP, hit frequency, bonus frequency and payout, volatility and
// max win. Bonuses are played to completion by a bot that picks random valid
// actions, so player-choice bonuses must have the same expected value for
// every choice (or the report reflects random play).
package sim

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
)

type Options struct {
	Spins   int64
	Bet     int64 // defaults to the game's default bet
	Workers int   // defaults to GOMAXPROCS
	// NewRNG makes one RNG per worker. Defaults to the crypto RNG.
	NewRNG func() games.RNG
	// Progress, if set, is called about once a second with spins done.
	Progress func(done int64)
}

// WinBuckets are the upper bounds (in bets, exclusive) of the win histogram.
// The last bucket is open-ended.
var WinBuckets = []float64{0, 1, 2, 5, 10, 25, 50, 100, 500, math.Inf(1)}

type Report struct {
	GameID   string
	Spins    int64
	Bet      int64
	Duration time.Duration

	TotalBet  int64
	BaseWin   int64
	BonusWin  int64
	Hits      int64 // spins with any win, including bonus
	Bonuses   int64
	BonusActs int64
	MaxWin    int64 // single spin incl. its bonus, in Coins

	// Histogram counts spins by total win in bets: [0], (0,1), [1,2), ... [500,∞).
	Histogram []int64

	sumR, sumR2 float64 // sum of win/bet and its square, per spin
}

func (r *Report) TotalWin() int64 { return r.BaseWin + r.BonusWin }

// RTP is total win over total bet.
func (r *Report) RTP() float64 { return ratio(r.TotalWin(), r.TotalBet) }

func (r *Report) BaseRTP() float64  { return ratio(r.BaseWin, r.TotalBet) }
func (r *Report) BonusRTP() float64 { return ratio(r.BonusWin, r.TotalBet) }

// HitFrequency is the share of spins that win anything.
func (r *Report) HitFrequency() float64 { return ratio(r.Hits, r.Spins) }

// BonusFrequency is the share of spins that trigger the bonus (1 in 1/x spins).
func (r *Report) BonusFrequency() float64 { return ratio(r.Bonuses, r.Spins) }

// AvgBonusPayout is the mean bonus win in bets (excluding the triggering spin).
func (r *Report) AvgBonusPayout() float64 {
	if r.Bonuses == 0 {
		return 0
	}
	return float64(r.BonusWin) / float64(r.Bet) / float64(r.Bonuses)
}

// StdDev is the standard deviation of a spin's return, in bets.
func (r *Report) StdDev() float64 {
	if r.Spins < 2 {
		return 0
	}
	n := float64(r.Spins)
	mean := r.sumR / n
	return math.Sqrt(math.Max(0, (r.sumR2-n*mean*mean)/(n-1)))
}

// VolatilityIndex is 1.96σ: the usual 95%-confidence volatility index.
func (r *Report) VolatilityIndex() float64 { return 1.96 * r.StdDev() }

// RTPConfidence is the 95% confidence half-width of the measured RTP.
func (r *Report) RTPConfidence() float64 {
	if r.Spins == 0 {
		return 0
	}
	return 1.96 * r.StdDev() / math.Sqrt(float64(r.Spins))
}

// MaxWinX is the largest single-spin win in bets.
func (r *Report) MaxWinX() float64 { return ratio(r.MaxWin, r.Bet) }

func ratio(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func (r *Report) merge(o *Report) {
	r.TotalBet += o.TotalBet
	r.BaseWin += o.BaseWin
	r.BonusWin += o.BonusWin
	r.Hits += o.Hits
	r.Bonuses += o.Bonuses
	r.BonusActs += o.BonusActs
	r.MaxWin = max(r.MaxWin, o.MaxWin)
	r.sumR += o.sumR
	r.sumR2 += o.sumR2
	for i, c := range o.Histogram {
		r.Histogram[i] += c
	}
}

// maxBonusActions bounds one bonus, to catch state machines that never finish.
const maxBonusActions = 100_000

const chunk = 4096

// Run simulates opts.Spins spins of game.
func Run(ctx context.Context, game games.SlotGame, opts Options) (*Report, error) {
	info := game.Info()
	if opts.Bet == 0 {
		opts.Bet = info.DefaultBet
	}
	if opts.Bet <= 0 {
		return nil, errors.New("sim: bet must be positive")
	}
	if opts.Workers <= 0 {
		opts.Workers = runtime.GOMAXPROCS(0)
	}
	if opts.NewRNG == nil {
		opts.NewRNG = func() games.RNG { return games.NewCryptoRNG() }
	}

	start := time.Now()
	var next, done atomic.Int64
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	reports := make([]*Report, opts.Workers)
	var wg sync.WaitGroup
	for w := range opts.Workers {
		rep := newReport(info.Id, opts.Bet)
		reports[w] = rep
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := opts.NewRNG()
			for ctx.Err() == nil {
				from := next.Add(chunk) - chunk
				if from >= opts.Spins {
					return
				}
				n := min(int64(chunk), opts.Spins-from)
				for range n {
					if err := spinOnce(game, rng, opts.Bet, rep); err != nil {
						cancel(err)
						return
					}
				}
				done.Add(n)
			}
		}()
	}

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	if opts.Progress != nil {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
	loop:
		for {
			select {
			case <-finished:
				break loop
			case <-tick.C:
				opts.Progress(done.Load())
			}
		}
	}
	<-finished

	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	total := newReport(info.Id, opts.Bet)
	for _, r := range reports {
		total.merge(r)
	}
	total.Spins = done.Load()
	total.Duration = time.Since(start)
	return total, nil
}

func newReport(id string, bet int64) *Report {
	return &Report{GameID: id, Bet: bet, Histogram: make([]int64, len(WinBuckets))}
}

func spinOnce(game games.SlotGame, rng games.RNG, bet int64, rep *Report) error {
	out, err := game.Spin(rng, bet)
	if err != nil {
		return fmt.Errorf("spin: %w", err)
	}
	win := out.Outcome.TotalWin
	rep.TotalBet += bet
	rep.BaseWin += win

	if out.Trigger != nil {
		rep.Bonuses++
		bonusWin, acts, err := PlayBonus(game, rng, bet, *out.Trigger)
		if err != nil {
			return err
		}
		rep.BonusActs += int64(acts)
		rep.BonusWin += bonusWin
		win += bonusWin
	}

	if win > 0 {
		rep.Hits++
	}
	rep.MaxWin = max(rep.MaxWin, win)
	x := float64(win) / float64(bet)
	rep.sumR += x
	rep.sumR2 += x * x
	for i, ub := range WinBuckets {
		if (i == 0 && win == 0) || (i > 0 && x < ub) {
			rep.Histogram[i]++
			break
		}
	}
	return nil
}

// PlayBonus plays a bonus to completion with random valid actions and returns
// its total win and the number of actions taken.
func PlayBonus(game games.SlotGame, rng games.RNG, bet int64, trigger api.BonusTrigger) (int64, int, error) {
	state, err := game.StartBonus(rng, bet, trigger)
	if err != nil {
		return 0, 0, fmt.Errorf("start bonus: %w", err)
	}
	var total int64
	for acts := 0; !state.Done; acts++ {
		if acts >= maxBonusActions {
			return 0, acts, fmt.Errorf("bonus did not finish after %d actions", acts)
		}
		action, ok := ChooseAction(rng, state)
		if !ok {
			return 0, acts, fmt.Errorf("bonus is not done but offers no actions (kind %s)", state.Kind)
		}
		next, step, err := game.BonusAction(rng, state, action)
		if err != nil {
			return 0, acts, fmt.Errorf("bonus action %+v: %w", action, err)
		}
		if step.Win < 0 {
			return 0, acts, fmt.Errorf("bonus action returned negative win %d", step.Win)
		}
		total += step.Win
		state = next
		if state.Done {
			return total, acts + 1, nil
		}
	}
	return total, 0, nil
}

// ChooseAction is the simulator's player: a random offered action, with a
// random choice when the action takes one.
func ChooseAction(rng games.RNG, state games.BonusState) (games.Action, bool) {
	if len(state.Actions) == 0 {
		return games.Action{}, false
	}
	opt := state.Actions[rng.IntN(len(state.Actions))]
	a := games.Action{Name: opt.Action}
	if len(opt.Choices) > 0 {
		a.Choice = opt.Choices[rng.IntN(len(opt.Choices))]
	}
	return a, true
}
