package eyeofra

import (
	"encoding/json"
	"errors"
	"strconv"
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

var alias = map[byte]string{
	'l': "lotus", 'a': "ankh", 'd': "djed", 'f': "feather",
	'J': "jar", 'C': "cat", 'F': "falcon", 'M': "mask", 'E': "eye",
}

// grid draws a 6×5 board from five rows of six characters.
func grid(g *Game, rows ...string) games.Grid {
	out := games.NewGrid(g.Heights)
	for row, line := range rows {
		cells := strings.ReplaceAll(line, " ", "")
		for reel := range len(cells) {
			out[reel][row] = g.Symbols.MustLookup(alias[cells[reel]])
		}
	}
	return out
}

// noCluster has no group of 5 touching identical symbols.
var noCluster = []string{
	"l a d f J C",
	"a d f J C F",
	"d f J C F M",
	"f J C F M l",
	"J C F M l a",
}

func TestConfigIsValid(t *testing.T) {
	g := load(t)
	info := g.Info()
	if info.Id != ID || info.Reels != 6 || info.Rows != 5 || info.Layout != api.Cluster {
		t.Fatalf("info: %+v", info)
	}
	if len(g.chambers) != 3 || g.chambers[0].curses != 0 || !g.chambers[0].passage || g.chambers[2].passage || g.chambers[2].pharaoh == 0 {
		t.Fatalf("chambers: %+v", g.chambers)
	}
}

func TestMultiplierLadder(t *testing.T) {
	g := load(t)
	for step, want := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 10, 10} {
		if got := g.multiplierFor(step); got != want {
			t.Errorf("step %d: multiplier %d, want %d", step, got, want)
		}
	}
}

func TestTrigger(t *testing.T) {
	g := load(t)
	rng := &gamestest.ScriptedRNG{}
	tests := []struct {
		name      string
		rows      []string
		triggered bool
		extra     int
	}{
		{"no scatters", noCluster, false, 0},
		{"three scatters", []string{"E a d f J C", "a d f J E F", "d f J C F M", "f J C F M l", "J C E M l a"}, false, 0},
		{"four scatters", []string{"E a d f J C", "a d f J E F", "d f J C F M", "f J C F M E", "J C E M l a"}, true, 0},
		{"six scatters raise every chamber multiplier by 2", []string{"E a d f J E", "a d f J E F", "d f J C F M", "f E C F M E", "J C E M l a"}, true, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := g.spinGrid(rng, grid(g, tc.rows...), 100)
			if (out.Trigger != nil) != tc.triggered {
				t.Fatalf("trigger %+v, want triggered=%v", out.Trigger, tc.triggered)
			}
			if out.Trigger != nil && out.Trigger.Data["bonusMultiplier"] != tc.extra {
				t.Fatalf("trigger data %v, want bonusMultiplier %d", out.Trigger.Data, tc.extra)
			}
		})
	}
}

func TestClusterWinOnTheFirstBoard(t *testing.T) {
	g := load(t)
	// Five masks in an L-shape; nothing else pays.
	rows := []string{
		"M M M f J C",
		"M d f J C F",
		"M f J C F M",
		"f J C F M l",
		"J C F M l a",
	}
	out := g.spinGrid(games.NewCryptoRNG(), grid(g, rows...), 100)
	first := out.Outcome.Steps[0]
	mask := g.Symbols.MustLookup("mask")
	want := g.Pays.Pay(mask, 5).Coins(100)
	if len(first.Wins) != 1 || first.Wins[0].Symbol != "mask" || first.Wins[0].Count != 5 || first.StepWin != want || first.Multiplier != 0 {
		t.Fatalf("first step: %+v, want one 5-mask cluster paying %d at ×1", first, want)
	}
	if len(first.Removed) != 5 || len(out.Outcome.Steps) < 2 {
		t.Fatalf("the cluster must be removed and the board must tumble: %+v", out.Outcome.Steps)
	}
}

// TestSpinInvariants checks real spins: every step's pay matches its wins at
// the cascade multiplier, the total adds up, and the trigger appears exactly
// when the final board holds enough scatters.
func TestSpinInvariants(t *testing.T) {
	g := load(t)
	rng := games.NewCryptoRNG()
	sawCascade := false
	for range 5000 {
		out, err := g.Spin(rng, 100)
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for i, st := range out.Outcome.Steps {
			m := g.multiplierFor(i)
			wantMult := m
			if m == 1 {
				wantMult = 0
			}
			if st.Multiplier != wantMult {
				t.Fatalf("step %d multiplier %d, want %d", i, st.Multiplier, wantMult)
			}
			var sum int64
			for _, w := range st.Wins {
				s := g.Symbols.MustLookup(w.Symbol)
				if w.Count < 5 || w.Amount != (g.Pays.Pay(s, w.Count)*games.Units(m)).Coins(100) {
					t.Fatalf("step %d win %+v does not match the paytable at ×%d", i, w, m)
				}
				sum += w.Amount
			}
			if sum != st.StepWin {
				t.Fatalf("step %d: wins sum to %d, step win %d", i, sum, st.StepWin)
			}
			last := i == len(out.Outcome.Steps)-1
			if last != (len(st.Removed) == 0) {
				t.Fatalf("step %d: removed %d cells (last=%v)", i, len(st.Removed), last)
			}
			if i > 0 {
				sawCascade = true
			}
			total += st.StepWin
		}
		if total != out.Outcome.TotalWin {
			t.Fatalf("steps sum to %d, total %d", total, out.Outcome.TotalWin)
		}
		final := out.Outcome.Steps[len(out.Outcome.Steps)-1].Grid
		eyes := 0
		for _, reel := range final {
			for _, s := range reel {
				if s == "eye" {
					eyes++
				}
			}
		}
		if (eyes >= g.p.TriggerCount) != (out.Trigger != nil) {
			t.Fatalf("%d eyes on the final board, trigger %v", eyes, out.Trigger)
		}
	}
	if !sawCascade {
		t.Fatal("5000 spins without a cascade")
	}
}

func start(t *testing.T, g *Game, extra int) games.BonusState {
	t.Helper()
	bs, err := g.StartBonus(games.NewCryptoRNG(), 100, api.BonusTrigger{Kind: api.Pick, Data: map[string]any{"bonusMultiplier": extra}})
	if err != nil {
		t.Fatal(err)
	}
	return bs
}

func decode(t *testing.T, bs games.BonusState) state {
	t.Helper()
	var s state
	if err := json.Unmarshal(bs.Private, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTombLayout(t *testing.T) {
	g := load(t)
	bs := start(t, g, 1)
	s := decode(t, bs)
	for c, urns := range s.Urns {
		count := map[string]int{}
		for _, u := range urns {
			count[u.Kind]++
		}
		ch := g.chambers[c]
		wantPassage := map[bool]int{true: 1}[ch.passage]
		wantPharaoh := map[bool]int{true: 1}[ch.pharaoh > 0]
		if len(urns) != ch.urns || count[Curse] != ch.curses || count[Passage] != wantPassage || count[Pharaoh] != wantPharaoh ||
			count[Treasure] != ch.urns-ch.curses-wantPassage-wantPharaoh {
			t.Fatalf("chamber %d layout %v", c+1, count)
		}
	}
	// Nothing about the sealed urns reaches the client.
	pub, _ := json.Marshal(bs.Public)
	for _, leak := range []string{Curse, Passage, Pharaoh, Treasure, "value"} {
		if strings.Contains(string(pub), leak) {
			t.Fatalf("public state leaks %q: %s", leak, pub)
		}
	}
	if mults := bs.Public["multipliers"].([]int); mults[0] != 2 || mults[2] != 4 {
		t.Fatalf("extra scatter multiplier not applied: %v", mults)
	}
	if len(bs.Actions) != 1 || bs.Actions[0].Action != "pick" || len(bs.Actions[0].Choices) != 9 {
		t.Fatalf("actions %+v", bs.Actions)
	}
}

func TestTombPlaysThrough(t *testing.T) {
	g := load(t)
	rng := games.NewCryptoRNG()
	endings := map[string]int{}
	for range 2000 {
		bs := start(t, g, 0)
		var total int64
		for !bs.Done {
			a := bs.Actions[0]
			choice := a.Choices[rng.IntN(len(a.Choices))]
			before := decode(t, bs)
			next, step, err := g.BonusAction(rng, bs, games.Action{Name: "pick", Choice: choice})
			if err != nil {
				t.Fatal(err)
			}
			i, _ := strconv.Atoi(choice)
			u := before.Urns[before.Chamber][i]
			switch u.Kind {
			case Curse, Passage:
				if step.Win != 0 {
					t.Fatalf("%s paid %d", u.Kind, step.Win)
				}
			case Treasure, Pharaoh:
				want := (u.Value * games.Units(g.chamberMultiplier(before, before.Chamber))).Coins(100)
				if step.Win != want && !next.Done {
					t.Fatalf("%s paid %d, want %d", u.Kind, step.Win, want)
				}
			}
			if u.Kind == Curse && before.Chamber == 0 {
				t.Fatal("a curse in the first chamber")
			}
			total += step.Win
			bs = next
		}
		s := decode(t, bs)
		if s.Win.Coins(100) != total {
			t.Fatalf("private win %d, sum of steps %d", s.Win.Coins(100), total)
		}
		endings[s.Ending]++
		// At the end every urn is shown.
		pub, _ := json.Marshal(bs.Public)
		if strings.Contains(string(pub), Hidden) {
			t.Fatalf("finished bonus still hides urns: %s", pub)
		}
	}
	if endings[Curse] == 0 || endings["cleared"] == 0 {
		t.Fatalf("expected both curses and cleared tombs in 2000 bonuses: %v", endings)
	}
}

func TestTombRules(t *testing.T) {
	g := load(t)
	// A hand-built tomb, so each rule is checked exactly.
	s := state{Bet: 100, Urns: [][]urn{
		{{Kind: Treasure, Value: 20}, {Kind: Passage}},
		{{Kind: Treasure, Value: 20}, {Kind: Curse}, {Kind: Passage}},
		{{Kind: Pharaoh, Value: 400}, {Kind: Treasure, Value: 20}, {Kind: Curse}},
	}}
	bs, _ := g.bonusState(s)
	pick := func(i int) games.BonusStep {
		t.Helper()
		next, step, err := g.BonusAction(nil, bs, games.Action{Name: "pick", Choice: strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		bs = next
		return step
	}
	if step := pick(0); step.Win != (20 * games.Units(g.chambers[0].multiplier)).Coins(100) {
		t.Fatalf("treasure paid %d", step.Win)
	}
	for _, bad := range []games.Action{{Name: "pick", Choice: "0"}, {Name: "pick", Choice: "7"}, {Name: "pick", Choice: "x"}, {Name: "spin"}} {
		if _, _, err := g.BonusAction(nil, bs, bad); !errors.Is(err, games.ErrInvalidAction) {
			t.Fatalf("%+v: got %v, want ErrInvalidAction", bad, err)
		}
	}
	if step := pick(1); step.Reveal["kind"] != Passage || bs.Public["chamber"] != 1 {
		t.Fatalf("passage did not advance: %+v %v", step.Reveal, bs.Public["chamber"])
	}
	pick(2) // passage in chamber 2
	if bs.Public["chamber"] != 2 {
		t.Fatal("second passage did not advance")
	}
	pick(0) // Pharaoh's treasure
	if bs.Done {
		t.Fatal("ended with a treasure still sealed")
	}
	step := pick(1) // last treasure: only the curse is left, so the tomb is cleared
	if !bs.Done || step.Reveal["ending"] != "cleared" {
		t.Fatalf("tomb not cleared: done=%v reveal=%v", bs.Done, step.Reveal)
	}

	// A curse ends the bonus and keeps what was won.
	s = state{Bet: 100, Chamber: 1, Win: 60, Urns: [][]urn{{{Kind: Passage, Revealed: true}}, {{Kind: Curse}, {Kind: Passage}}, {{Kind: Curse}}}}
	bs, _ = g.bonusState(s)
	step = pick(0)
	if !bs.Done || step.Win != 0 || step.Reveal["ending"] != Curse || decode(t, bs).Win != 60 {
		t.Fatalf("curse: done=%v step=%+v", bs.Done, step)
	}
}

func TestTombWinCap(t *testing.T) {
	g := load(t)
	limit := games.Units(g.p.MaxWinX * games.UnitsPerBet)
	s := state{Bet: 100, Chamber: 0, Win: limit - 5, Urns: [][]urn{{{Kind: Treasure, Value: 400}, {Kind: Passage}}, {{Kind: Passage}}, {{Kind: Curse}}}}
	bs, _ := g.bonusState(s)
	next, step, err := g.BonusAction(nil, bs, games.Action{Name: "pick", Choice: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if !next.Done || step.Win != games.Units(5).Coins(100) || step.Reveal["ending"] != EventMaxWin {
		t.Fatalf("cap: done=%v step=%+v", next.Done, step)
	}
}
