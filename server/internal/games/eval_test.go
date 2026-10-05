package games_test

import (
	"slices"
	"strings"
	"testing"

	"pantheon-spins/server/internal/games"
)

// Test symbols: one character each, so grids can be drawn as text.
//
//	a b c  low      h  high     w  wild     s  scatter     x  special (never pays)
var testSymbols = games.NewSymbols([]games.SymbolDef{
	{ID: "a", Kind: games.KindLow},
	{ID: "b", Kind: games.KindLow},
	{ID: "c", Kind: games.KindLow},
	{ID: "h", Kind: games.KindHigh},
	{ID: "w", Kind: games.KindWild},
	{ID: "s", Kind: games.KindScatter},
	{ID: "x", Kind: games.KindSpecial},
})

func sym(id string) games.Symbol { return testSymbols.MustLookup(id) }

// grid draws a grid from rows of text, top row first, one character per reel.
// Spaces are ignored. A '.' leaves a cell off the reel, for ragged grids
// (cells must then be at the bottom of each reel… or the top for hex tests);
// it is only allowed at the end of a column.
func grid(rows ...string) games.Grid {
	clean := make([]string, len(rows))
	for i, r := range rows {
		clean[i] = strings.ReplaceAll(r, " ", "")
	}
	reels := len(clean[0])
	g := make(games.Grid, reels)
	for _, row := range clean {
		for r := 0; r < reels; r++ {
			if c := string(row[r]); c != "." {
				g[r] = append(g[r], sym(c))
			}
		}
	}
	return g
}

// paytable builds a paytable in units from symbol → count → units.
func paytable(p map[string]map[int]games.Units) games.Paytable {
	pt := make(games.Paytable, testSymbols.Len())
	for id, counts := range p {
		maxN := 0
		for n := range counts {
			maxN = max(maxN, n)
		}
		row := make([]games.Units, maxN+1)
		var cur games.Units
		for n := 1; n <= maxN; n++ {
			if u, ok := counts[n]; ok {
				cur = u
			}
			row[n] = cur
		}
		pt[sym(id)] = row
	}
	return pt
}

type winSummary struct {
	Symbol string
	Count  int
	Ways   int
	Line   int
	Pay    games.Units
	Mult   int
	Cells  int
}

func summarize(wins []games.Win) []winSummary {
	out := make([]winSummary, len(wins))
	for i, w := range wins {
		out[i] = winSummary{testSymbols.ID(w.Symbol), w.Count, w.Ways, w.Line, w.Pay, w.Multiplier, len(w.Positions)}
	}
	slices.SortFunc(out, func(a, b winSummary) int {
		if c := strings.Compare(a.Symbol, b.Symbol); c != 0 {
			return c
		}
		return a.Line - b.Line
	})
	return out
}

func multAt(cells map[games.Pos]int) games.CellMultiplier {
	if cells == nil {
		return nil
	}
	return func(p games.Pos) int { return cells[p] }
}

func TestWays(t *testing.T) {
	ways := games.Ways{
		Symbols: testSymbols,
		Pays: paytable(map[string]map[int]games.Units{
			"a": {3: 2, 4: 5, 5: 10},
			"b": {3: 3, 4: 6, 5: 12},
			"h": {3: 10, 4: 20, 5: 50, 6: 100},
		}),
	}
	tests := []struct {
		name string
		g    games.Grid
		mult map[games.Pos]int
		want []winSummary
	}{
		{
			name: "three of a kind, one way",
			g:    grid("a a a b c", "c c c c b", "h c c c c"),
			want: []winSummary{{"a", 3, 1, 0, 2, 1, 3}},
		},
		{
			name: "ways multiply across reels",
			g:    grid("a a a c c", "a c a c c", "c a a c c"),
			want: []winSummary{{"a", 3, 2 * 2 * 3, 0, 2 * 12, 1, 7}},
		},
		{
			name: "chain must start on the first reel",
			g:    grid("c a a a a", "c c c c c", "c c c c c"),
			want: nil,
		},
		{
			name: "wild substitutes on any reel",
			g:    grid("w w a c c", "c c c c c", "c c c c c"),
			want: []winSummary{{"a", 3, 1, 0, 2, 1, 3}},
		},
		{
			name: "wilds on the first reels feed every symbol",
			g:    grid("w w a b c", "c c c c c", "c c b c c"),
			want: []winSummary{{"a", 3, 1, 0, 2, 1, 3}, {"b", 4, 1, 0, 6, 1, 4}},
		},
		{
			name: "five of a kind",
			g:    grid("h h h h h", "c c c c c", "c c c c c"),
			want: []winSummary{{"h", 5, 1, 0, 50, 1, 5}},
		},
		{
			name: "scatter breaks the chain",
			g:    grid("a a s a a", "c c c c c", "c c c c c"),
			want: nil,
		},
		{
			name: "uniform multiplier is reported",
			g:    grid("a w a c c", "c c c c c", "c c c c c"),
			mult: map[games.Pos]int{{1, 0}: 3},
			want: []winSummary{{"a", 3, 1, 0, 6, 3, 3}},
		},
		{
			name: "multipliers sum within a reel and multiply across reels",
			// reel 1 has a plain 'a' (×1) and a wild ×2: 1+2 = 3 weighted ways, 2 plain ways.
			g:    grid("a a a c c", "c w c c c", "c c c c c"),
			mult: map[games.Pos]int{{1, 1}: 2},
			want: []winSummary{{"a", 3, 2, 0, 2 * 3, 1, 4}},
		},
		{
			name: "megaways: reels of different heights",
			g: grid(
				"h h h h h h",
				"h c h c . h",
				". c h . . c",
				". . h . . .",
			),
			want: []winSummary{{"h", 6, 1 * 2 * 4 * 1 * 1 * 2, 0, 100 * 16, 1, 11}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(ways.Evaluate(tc.g, multAt(tc.mult)))
			if !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestLines(t *testing.T) {
	lines := games.Lines{
		Symbols: testSymbols,
		Pays: paytable(map[string]map[int]games.Units{
			"a": {3: 2, 4: 4, 5: 8},
			"h": {3: 10, 4: 20, 5: 40},
			"w": {3: 30, 4: 60, 5: 200},
			"s": {3: 100}, // scatter pays never apply on lines
		}),
		Lines: [][]int{
			{1, 1, 1, 1, 1}, // 1: middle
			{0, 0, 0, 0, 0}, // 2: top
			{2, 2, 2, 2, 2}, // 3: bottom
			{0, 1, 2, 1, 0}, // 4: V
		},
	}
	tests := []struct {
		name string
		g    games.Grid
		mult map[games.Pos]int
		want []winSummary
	}{
		{
			name: "three on the middle line",
			g:    grid("c b c b c", "a a a c b", "b c b c b"),
			want: []winSummary{{"a", 3, 0, 1, 2, 1, 3}},
		},
		{
			name: "two is not enough",
			g:    grid("c b c b c", "a a c a b", "b c b c b"),
			want: nil,
		},
		{
			name: "V-shaped line",
			g:    grid("h b c b h", "c h b h c", "b c h c b"),
			want: []winSummary{{"h", 5, 0, 4, 40, 1, 5}},
		},
		{
			name: "leading wild takes the first real symbol",
			g:    grid("c b c b c", "w a a a b", "b c b c b"),
			want: []winSummary{{"a", 4, 0, 1, 4, 1, 4}},
		},
		{
			name: "wild-only run beats the substituted symbol when it pays more",
			g:    grid("c b c b c", "w w w a b", "b c b c b"),
			want: []winSummary{{"w", 3, 0, 1, 30, 1, 3}},
		},
		{
			name: "line of only wilds",
			g:    grid("c b c b c", "w w w w w", "b c b c b"),
			want: []winSummary{{"w", 5, 0, 1, 200, 1, 5}},
		},
		{
			name: "scatter on a line does not pay as a line",
			g:    grid("c b c b c", "s s s c b", "b c b c b"),
			want: nil,
		},
		{
			name: "multipliers on matched cells multiply",
			g:    grid("c b c b c", "a w w c b", "b c b c b"),
			mult: map[games.Pos]int{{1, 1}: 2, {2, 1}: 3, {3, 1}: 10},
			want: []winSummary{{"a", 3, 0, 1, 2 * 6, 6, 3}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(lines.Evaluate(tc.g, multAt(tc.mult)))
			if !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestClustersSquare(t *testing.T) {
	cl := games.Clusters{
		Symbols:  testSymbols,
		Topology: games.Square{},
		Pays: paytable(map[string]map[int]games.Units{
			"a": {5: 4, 6: 6, 8: 10}, // 7 pays like 6, 8+ pays 10
			"b": {5: 5},
		}),
	}
	tests := []struct {
		name string
		g    games.Grid
		mult map[games.Pos]int
		want []winSummary
	}{
		{
			name: "five connected",
			g:    grid("a a c c", "a c c c", "a a c c", "c c c c"),
			want: []winSummary{{"a", 5, 0, 0, 4, 1, 5}},
		},
		{
			name: "diagonals do not connect",
			g:    grid("a c a c", "c a c a", "a c a c", "c a c a"),
			want: nil,
		},
		{
			name: "four is too small",
			g:    grid("a a c c", "a a c c", "c c c c", "c c c c"),
			want: nil,
		},
		{
			name: "wild bridges two groups into one cluster",
			g:    grid("a a w a a", "c c c c a", "c c c c c"),
			want: []winSummary{{"a", 6, 0, 0, 6, 1, 6}},
		},
		{
			name: "a wild can serve clusters of different symbols",
			g:    grid("a a w b b", "a a c b b", "c c c c c"),
			want: []winSummary{{"a", 5, 0, 0, 4, 1, 5}, {"b", 5, 0, 0, 5, 1, 5}},
		},
		{
			name: "only wilds do not pay",
			g:    grid("w w w c", "w w c c", "c c c c"),
			want: nil,
		},
		{
			name: "size between table entries pays the lower bracket",
			g:    grid("a a a a", "a a a c", "c c c c"),
			want: []winSummary{{"a", 7, 0, 0, 6, 1, 7}},
		},
		{
			name: "size beyond the table pays the top entry",
			g:    grid("a a a a", "a a a a", "a a a a"),
			want: []winSummary{{"a", 12, 0, 0, 10, 1, 12}},
		},
		{
			name: "cell multipliers multiply the cluster",
			g:    grid("a a c c", "a w c c", "a c c c"),
			mult: map[games.Pos]int{{1, 1}: 5},
			want: []winSummary{{"a", 5, 0, 0, 20, 5, 5}},
		},
		{
			name: "separate clusters of one symbol pay separately",
			g:    grid("a a a c b b b", "a a c c b b c", "c c c c c c c"),
			want: []winSummary{{"a", 5, 0, 0, 4, 1, 5}, {"b", 5, 0, 0, 5, 1, 5}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(cl.Evaluate(tc.g, multAt(tc.mult)))
			if !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestHexNeighbors(t *testing.T) {
	// Honeycomb with column heights 5,4,5,4,5 (odd columns sit half a cell lower).
	g := games.NewGrid([]int{5, 4, 5, 4, 5})
	tests := []struct {
		name string
		p    games.Pos
		want []games.Pos
	}{
		{"even column, interior", games.Pos{2, 2}, []games.Pos{{2, 1}, {2, 3}, {1, 1}, {1, 2}, {3, 1}, {3, 2}}},
		{"odd column, interior", games.Pos{1, 1}, []games.Pos{{1, 0}, {1, 2}, {0, 1}, {0, 2}, {2, 1}, {2, 2}}},
		{"even column, top edge", games.Pos{0, 0}, []games.Pos{{0, 1}, {1, 0}}},
		{"odd column, bottom edge", games.Pos{3, 3}, []games.Pos{{3, 2}, {2, 3}, {2, 4}, {4, 3}, {4, 4}}},
		{"even column, bottom edge of a tall column", games.Pos{4, 4}, []games.Pos{{4, 3}, {3, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := games.Hex{}.Neighbors(g, tc.p, nil)
			slices.SortFunc(got, cmpPos)
			want := slices.Clone(tc.want)
			slices.SortFunc(want, cmpPos)
			if !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func cmpPos(a, b games.Pos) int {
	if a.Reel != b.Reel {
		return a.Reel - b.Reel
	}
	return a.Row - b.Row
}

func TestClustersHex(t *testing.T) {
	cl := games.Clusters{
		Symbols:  testSymbols,
		Topology: games.Hex{},
		Pays:     paytable(map[string]map[int]games.Units{"a": {4: 3}}),
	}
	// Column-major so the hex shape is explicit: heights 3,3,3,3.
	col := func(cols ...string) games.Grid {
		g := make(games.Grid, len(cols))
		for r, c := range cols {
			for _, ch := range c {
				g[r] = append(g[r], sym(string(ch)))
			}
		}
		return g
	}
	tests := []struct {
		name string
		g    games.Grid
		want []winSummary
	}{
		{
			// (0,1),(1,1),(2,2),(3,2): odd column 1 touches rows 1 and 2 of
			// column 2, so this staircase is one cluster on hex. On a square
			// grid (1,1)–(2,2) is a diagonal and it splits into two pairs.
			name: "hex diagonal staircase connects",
			g:    col("cac", "cac", "cca", "cca"),
			want: []winSummary{{"a", 4, 0, 0, 3, 1, 4}},
		},
		{
			// Even column 0 touches rows 0 and 1 of column 1, not row 2.
			name: "the other diagonal does not connect",
			g:    col("cac", "cca", "cca", "cca"),
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(cl.Evaluate(tc.g, nil))
			if !slices.Equal(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
	// Sanity check: the first grid has no square-topology cluster of 4.
	sq := games.Clusters{Symbols: testSymbols, Topology: games.Square{}, Pays: cl.Pays}
	if wins := sq.Evaluate(tests[0].g, nil); len(wins) != 0 {
		t.Fatalf("square topology unexpectedly found %+v", summarize(wins))
	}
}

func TestTumble(t *testing.T) {
	tests := []struct {
		name    string
		g       games.Grid
		removed []games.Pos
		want    games.Grid
	}{
		{
			name:    "survivors fall and keep order, gaps fill from the top",
			g:       grid("a b", "b c", "c a", "h b"),
			removed: []games.Pos{{0, 1}, {0, 3}},
			want:    grid("x b", "x c", "a a", "c b"),
		},
		{
			name:    "a cleared reel is fully refilled",
			g:       grid("a b", "a c"),
			removed: []games.Pos{{0, 0}, {0, 1}},
			want:    grid("x b", "x c"),
		},
		{
			name:    "nothing removed leaves the grid unchanged",
			g:       grid("a b", "c h"),
			removed: nil,
			want:    grid("a b", "c h"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.g.Clone()
			got := games.Tumble(tc.g, tc.removed, func(int, int) games.Symbol { return sym("x") })
			if !gridsEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if !gridsEqual(tc.g, before) {
				t.Fatal("Tumble modified its input")
			}
		})
	}
}

func gridsEqual(a, b games.Grid) bool {
	return slices.EqualFunc(a, b, func(x, y []games.Symbol) bool { return slices.Equal(x, y) })
}

func TestRunCascade(t *testing.T) {
	cl := games.Clusters{
		Symbols:  testSymbols,
		Topology: games.Square{},
		Pays:     paytable(map[string]map[int]games.Units{"a": {3: 2}, "b": {3: 3}}),
	}
	// Refill draws from a fixed queue: first a column of b's that completes a
	// second cluster, then neutral symbols.
	queue := []string{"b", "b", "b"}
	fill := func(int, int) games.Symbol {
		if len(queue) == 0 {
			return sym("c")
		}
		s := queue[0]
		queue = queue[1:]
		return sym(s)
	}
	eval := func(step int, g games.Grid) ([]games.Event, []games.Win, []games.Pos, int) {
		wins := cl.Evaluate(g, nil)
		return nil, wins, games.WinPositions(wins), step + 1 // multiplier grows 1, 2, 3…
	}
	start := grid(
		"c h c",
		"a h c",
		"a h c",
		"a h c",
	)
	steps := games.RunCascade(start, eval, fill)

	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}
	want := []struct {
		pay  games.Units
		mult int
		rm   int
	}{
		{2, 1, 3},     // a×3, multiplier 1
		{3 * 2, 2, 3}, // the refilled b×3, multiplier 2
		{0, 3, 0},     // nothing more
	}
	for i, w := range want {
		s := steps[i]
		if s.Pay != w.pay || s.Multiplier != w.mult || len(s.Removed) != w.rm {
			t.Fatalf("step %d: pay %d mult %d removed %d; want %d %d %d", i, s.Pay, s.Multiplier, len(s.Removed), w.pay, w.mult, w.rm)
		}
	}
	if got := games.CascadePay(steps); got != 8 {
		t.Fatalf("total pay %d, want 8", got)
	}
	if !gridsEqual(steps[0].Grid, start) {
		t.Fatal("first step must show the starting grid")
	}
	if steps[1].Grid[0][0] != sym("b") || steps[1].Grid[0][3] != sym("c") {
		t.Fatalf("second grid did not tumble as expected: %v", steps[1].Grid)
	}

	// API conversion: amounts in Coins at bet 100 (5 Coins per unit).
	api := testSymbols.StepsAPI(100, steps)
	if api[0].StepWin != 10 || api[1].StepWin != 30 || api[1].Multiplier != 2 || api[0].Multiplier != 0 {
		t.Fatalf("API steps: %+v", api)
	}
	if api[2].Removed != nil || len(api[0].Removed) != 3 || api[1].Wins[0].Symbol != "b" {
		t.Fatalf("API steps: %+v", api)
	}
}

func TestAnticipation(t *testing.T) {
	isS := func(s games.Symbol) bool { return s == sym("s") }
	tests := []struct {
		name string
		g    games.Grid
		need int
		want []int
	}{
		{"two scatters by reel 2 of 5", grid("s c s c c", "c c c c c"), 3, []int{3, 4}},
		{"two scatters on reel 0", grid("s c c c c", "s c c c c"), 3, []int{1, 2, 3, 4}},
		{"only one scatter", grid("s c c c c", "c c c c c"), 3, nil},
		{"second scatter on the last reel", grid("s c c c s", "c c c c c"), 3, nil},
		{"trigger completes on the next reel: still anticipated", grid("s s s c c", "c c c c c"), 3, []int{2, 3, 4}},
		{"enough scatters land on one reel", grid("s c c", "s c c", "s c c"), 3, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := games.Anticipation(tc.g, isS, tc.need); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
