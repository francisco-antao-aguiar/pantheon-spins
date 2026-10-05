// Command simulate measures a slot game's maths over many spins, in parallel
// across all CPU cores.
//
//	go run ./cmd/simulate -game halls-of-valhalla -spins 10000000
//	go run ./cmd/simulate -game all -spins 1000000 -check
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/catalog"
	"pantheon-spins/server/internal/sim"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "simulate:", err)
		os.Exit(1)
	}
}

// TargetRanger is implemented by games built on a config (via *games.Game).
type TargetRanger interface {
	TargetRange() (min, max float64)
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	gameID := fs.String("game", "", `game ID, or "all"`)
	spins := fs.Int64("spins", 10_000_000, "spins to simulate")
	bet := fs.Int64("bet", 0, "bet in Coins (default: the game's default bet)")
	workers := fs.Int("workers", runtime.NumCPU(), "parallel workers")
	configDir := fs.String("config-dir", envOr("GAME_CONFIG_DIR", "../game-configs"), "game config directory")
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	check := fs.Bool("check", false, "exit non-zero if RTP is outside the game's target range")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg := games.NewRegistry()
	if err := catalog.Register(reg, *configDir); err != nil {
		return err
	}
	var targets []games.SlotGame
	switch *gameID {
	case "":
		fs.Usage()
		return errors.New("-game is required; registered games: " + listIDs(reg))
	case "all":
		for _, info := range reg.List() {
			g, _ := reg.Get(info.Id)
			targets = append(targets, g)
		}
		if len(targets) == 0 {
			return errors.New("no games are registered yet")
		}
	default:
		g, ok := reg.Get(*gameID)
		if !ok {
			return fmt.Errorf("unknown game %q; registered games: %s", *gameID, listIDs(reg))
		}
		targets = append(targets, g)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var failed []string
	for _, g := range targets {
		id := g.Info().Id
		rep, err := sim.Run(ctx, g, sim.Options{
			Spins:   *spins,
			Bet:     *bet,
			Workers: *workers,
			Progress: func(done int64) {
				fmt.Fprintf(stderr, "\r%s: %5.1f%%", id, 100*float64(done)/float64(*spins))
			},
		})
		fmt.Fprint(stderr, "\r\033[K")
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		lo, hi, hasTarget := target(g)
		if *asJSON {
			writeJSON(stdout, rep, lo, hi)
		} else {
			writeTable(stdout, rep, *workers, lo, hi, hasTarget)
		}
		if *check && hasTarget && (rep.RTP() < lo || rep.RTP() > hi) {
			failed = append(failed, fmt.Sprintf("%s RTP %.2f%% outside %.1f–%.1f%%", id, 100*rep.RTP(), 100*lo, 100*hi))
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

func target(g games.SlotGame) (lo, hi float64, ok bool) {
	if t, isT := g.(TargetRanger); isT {
		lo, hi = t.TargetRange()
		return lo, hi, true
	}
	return 0, 0, false
}

func listIDs(reg *games.Registry) string {
	var ids []string
	for _, info := range reg.List() {
		ids = append(ids, info.Id)
	}
	if len(ids) == 0 {
		return "(none yet)"
	}
	return strings.Join(ids, ", ")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func oneIn(p float64) string {
	if p == 0 {
		return "never"
	}
	return fmt.Sprintf("1 in %.1f", 1/p)
}

func writeTable(w io.Writer, r *sim.Report, workers int, lo, hi float64, hasTarget bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	rate := float64(r.Spins) / r.Duration.Seconds()
	fmt.Fprintf(tw, "Game\t%s\n", r.GameID)
	fmt.Fprintf(tw, "Spins\t%d at bet %d (%s, %d workers, %.0f spins/s)\n", r.Spins, r.Bet, r.Duration.Round(time.Millisecond), workers, rate)
	status := ""
	if hasTarget {
		status = fmt.Sprintf("  target %.1f–%.1f%%", 100*lo, 100*hi)
		if r.RTP() >= lo && r.RTP() <= hi {
			status += "  OK"
		} else {
			status += "  OUT OF RANGE"
		}
	}
	fmt.Fprintf(tw, "RTP\t%.3f%% ± %.3f%% (95%%)%s\n", 100*r.RTP(), 100*r.RTPConfidence(), status)
	fmt.Fprintf(tw, "  base game\t%.3f%%\n", 100*r.BaseRTP())
	fmt.Fprintf(tw, "  bonus\t%.3f%%\n", 100*r.BonusRTP())
	fmt.Fprintf(tw, "Hit frequency\t%.2f%% (%s)\n", 100*r.HitFrequency(), oneIn(r.HitFrequency()))
	fmt.Fprintf(tw, "Bonus frequency\t%s spins (%d bonuses, %.1f actions avg)\n", oneIn(r.BonusFrequency()), r.Bonuses, avg(r.BonusActs, r.Bonuses))
	fmt.Fprintf(tw, "Avg bonus payout\t%.2f× bet\n", r.AvgBonusPayout())
	fmt.Fprintf(tw, "Volatility index\t%.2f (σ = %.2f× bet)\n", r.VolatilityIndex(), r.StdDev())
	fmt.Fprintf(tw, "Max win\t%.2f× bet\n", r.MaxWinX())
	fmt.Fprintf(tw, "Win distribution\t\n")
	labels := []string{"no win", "< 1×", "1–2×", "2–5×", "5–10×", "10–25×", "25–50×", "50–100×", "100–500×", "500×+"}
	for i, c := range r.Histogram {
		fmt.Fprintf(tw, "  %s\t%.4f%%\n", labels[i], 100*float64(c)/float64(max(r.Spins, 1)))
	}
	fmt.Fprintln(tw)
	tw.Flush()
}

func avg(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func writeJSON(w io.Writer, r *sim.Report, lo, hi float64) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"game":             r.GameID,
		"spins":            r.Spins,
		"bet":              r.Bet,
		"durationMs":       r.Duration.Milliseconds(),
		"rtp":              r.RTP(),
		"rtpConfidence95":  r.RTPConfidence(),
		"baseRtp":          r.BaseRTP(),
		"bonusRtp":         r.BonusRTP(),
		"targetRtp":        map[string]float64{"min": lo, "max": hi},
		"hitFrequency":     r.HitFrequency(),
		"bonusFrequency":   r.BonusFrequency(),
		"avgBonusPayoutX":  r.AvgBonusPayout(),
		"stdDevX":          r.StdDev(),
		"volatilityIndex":  r.VolatilityIndex(),
		"maxWinX":          r.MaxWinX(),
		"histogram":        r.Histogram,
		"histogramBuckets": []string{"0", "<1", "1-2", "2-5", "5-10", "10-25", "25-50", "50-100", "100-500", "500+"},
	})
}
