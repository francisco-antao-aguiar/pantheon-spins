package games_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
)

func loadSample(t *testing.T) *games.Game {
	t.Helper()
	g, err := games.LoadConfig("testdata", "sample-ways")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestLoadConfig(t *testing.T) {
	g := loadSample(t)
	info := g.Info()
	if info.Id != "sample-ways" || info.Layout != api.Ways || info.DefaultBet != 100 || len(info.Tags) != 1 {
		t.Fatalf("info: %+v", info)
	}
	if len(g.Heights) != 5 || g.Heights[0] != 3 {
		t.Fatalf("heights %v", g.Heights)
	}
	crown := g.Symbols.MustLookup("crown")
	if g.Pays.Pay(crown, 3) != 8 || g.Pays.Pay(crown, 5) != 60 || g.Pays.Pay(crown, 2) != 0 {
		t.Fatalf("crown pays in units: %v", g.Pays[crown])
	}
	if g.Symbols.Def(g.Symbols.MustLookup("ten")).Name != "10" {
		t.Fatal("symbol name not loaded")
	}
	base, err := g.ReelSet("base")
	if err != nil {
		t.Fatal(err)
	}
	if base[0].Weight(g.Symbols.MustLookup("wild")) != 0 || base[1].Weight(g.Symbols.MustLookup("wild")) != 4 {
		t.Fatal("reel weights not loaded")
	}
	if _, err := g.ReelSet("free"); err == nil {
		t.Fatal("missing reel set did not error")
	}
	var params struct{ Note string }
	if err := g.DecodeParams(&params); err != nil || params.Note == "" {
		t.Fatalf("params: %v %+v", err, params)
	}
	var strict struct{ Other string }
	if err := g.DecodeParams(&strict); err == nil {
		t.Fatal("unknown param field accepted")
	}
	if lo, hi := g.TargetRange(); lo != 0.94 || hi != 0.97 {
		t.Fatalf("target %v–%v", lo, hi)
	}
	if _, err := games.LoadConfig("testdata", "missing"); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestConfigValidation(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample-ways.json")
	if err != nil {
		t.Fatal(err)
	}
	// Each case edits the valid sample and expects an error mentioning want.
	tests := []struct {
		name string
		edit func(m map[string]any)
		want string
	}{
		{"bad id", func(m map[string]any) { m["id"] = "Bad ID" }, "id"},
		{"bad layout", func(m map[string]any) { m["layout"] = "spiral" }, "layout"},
		{"bad volatility", func(m map[string]any) { m["volatility"] = "spicy" }, "volatility"},
		{"bet not a multiple of 20", func(m map[string]any) { m["betLevels"] = []any{10, 20}; m["defaultBet"] = 20 }, "multiple of 20"},
		{"bets not increasing", func(m map[string]any) { m["betLevels"] = []any{40, 20}; m["defaultBet"] = 20 }, "increasing"},
		{"default bet not offered", func(m map[string]any) { m["defaultBet"] = 60 }, "defaultBet"},
		{"rtp target nonsense", func(m map[string]any) { m["targetRtp"] = map[string]any{"min": 0.97, "max": 0.94} }, "targetRtp"},
		{"pay not a multiple of 0.05", func(m map[string]any) { symbol(m, 0)["pays"] = map[string]any{"3": 0.07} }, "0.05"},
		{"pay count not a number", func(m map[string]any) { symbol(m, 0)["pays"] = map[string]any{"three": 1} }, "pay count"},
		{"duplicate symbol", func(m map[string]any) { symbol(m, 1)["id"] = "ten" }, "duplicate"},
		{"bad symbol kind", func(m map[string]any) { symbol(m, 0)["kind"] = "epic" }, "kind"},
		{"special symbol with pays", func(m map[string]any) {
			symbol(m, 6)["kind"] = "special"
			symbol(m, 6)["pays"] = map[string]any{"3": 1}
		}, "special"},
		{"no paying symbols", func(m map[string]any) {
			for i := range 5 {
				delete(symbol(m, i), "pays")
			}
		}, "must pay"},
		{"missing base reel set", func(m map[string]any) { m["reelSets"] = map[string]any{"free": reelSet(m)} }, `"base"`},
		{"wrong reel count", func(m map[string]any) { m["reelSets"].(map[string]any)["base"] = reelSet(m)[:4] }, "has 4 reels"},
		{"unknown symbol in reel set", func(m map[string]any) { reelSet(m)[0].(map[string]any)["dragon"] = 5 }, "unknown symbol"},
		{"empty reel", func(m map[string]any) { reelSet(m)[0] = map[string]any{} }, "empty"},
		{"lines on a ways game", func(m map[string]any) { m["lines"] = []any{[]any{1, 1, 1, 1, 1}} }, "lines"},
		{"lines layout without lines", func(m map[string]any) { m["layout"] = "lines" }, "requires lines"},
		{"line off the grid", func(m map[string]any) { m["layout"] = "lines"; m["lines"] = []any{[]any{1, 1, 3, 1, 1}} }, "off reel"},
		{"heights length", func(m map[string]any) { m["heights"] = []any{3, 3} }, "heights"},
		{"unknown field", func(m map[string]any) { m["colour"] = "red" }, "unknown field"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			tc.edit(m)
			edited, _ := json.Marshal(m)
			_, err := games.ParseConfig(edited)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func symbol(m map[string]any, i int) map[string]any {
	return m["symbols"].([]any)[i].(map[string]any)
}

func reelSet(m map[string]any) []any {
	return m["reelSets"].(map[string]any)["base"].([]any)
}

func TestUnits(t *testing.T) {
	tests := []struct {
		x       float64
		want    games.Units
		wantErr bool
	}{
		{0.05, 1, false}, {0.1, 2, false}, {1, 20, false}, {2.35, 47, false}, {500, 10000, false},
		{0, 0, false}, {0.07, 0, true}, {-1, 0, true},
	}
	for _, tc := range tests {
		got, err := games.UnitsFromMultiple(tc.x)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("UnitsFromMultiple(%v) = %d, %v", tc.x, got, err)
		}
	}
	// Coins are exact for every bet that is a multiple of 20.
	for _, bet := range []int64{20, 40, 100, 5000} {
		if got := games.Units(47).Coins(bet); got*games.UnitsPerBet != 47*bet {
			t.Errorf("Units(47).Coins(%d) = %d is not exact", bet, got)
		}
	}
}

func TestWeightTable(t *testing.T) {
	a, b, c := games.Symbol(0), games.Symbol(1), games.Symbol(2)
	wt, err := games.NewWeightTable(map[games.Symbol]int{a: 1, b: 0, c: 3})
	if err != nil {
		t.Fatal(err)
	}
	if wt.Total() != 4 || wt.Weight(c) != 3 || wt.Weight(b) != 0 {
		t.Fatalf("total %d weight(c) %d", wt.Total(), wt.Weight(c))
	}
	// Draw maps [0,1) to a and [1,4) to c (symbols in index order, b dropped).
	rng := &gamestest.ScriptedRNG{Values: []int{0, 1, 2, 3}}
	want := []games.Symbol{a, c, c, c}
	for i, w := range want {
		if got := wt.Draw(rng); got != w {
			t.Fatalf("draw %d = %d, want %d", i, got, w)
		}
	}
	if _, err := games.NewWeightTable(map[games.Symbol]int{a: 0}); err == nil {
		t.Fatal("empty table accepted")
	}
	if _, err := games.NewWeightTable(map[games.Symbol]int{a: -1}); err == nil {
		t.Fatal("negative weight accepted")
	}

	// Weighted picks an index in proportion to its weight.
	rng = &gamestest.ScriptedRNG{Values: []int{0, 1, 5, 6}}
	for i, want := range []int{0, 1, 1, 2} {
		if got := games.Weighted(rng, []int{1, 5, 4}); got != want {
			t.Fatalf("Weighted #%d = %d, want %d", i, got, want)
		}
	}
}
