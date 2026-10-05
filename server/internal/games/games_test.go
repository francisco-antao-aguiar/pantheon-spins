package games_test

import (
	"math"
	"testing"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
)

func TestCryptoRNGRangeAndUniformity(t *testing.T) {
	rng := games.NewCryptoRNG()
	for _, n := range []int{1, 2, 3, 7, 1000, math.MaxInt32} {
		for range 2000 {
			if v := rng.IntN(n); v < 0 || v >= n {
				t.Fatalf("IntN(%d) = %d out of range", n, v)
			}
		}
	}

	// Chi-squared test over 10 buckets. The critical value for 9 degrees of
	// freedom at p = 0.0001 is 33.7, so a false failure is very unlikely.
	const buckets, draws = 10, 200_000
	var counts [buckets]int
	for range draws {
		counts[rng.IntN(buckets)]++
	}
	expected := float64(draws) / buckets
	var chi2 float64
	for _, c := range counts {
		d := float64(c) - expected
		chi2 += d * d / expected
	}
	if chi2 > 33.7 {
		t.Fatalf("chi-squared %.1f too high: counts %v", chi2, counts)
	}
}

func TestCryptoRNGPanicsOnBadBound(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("IntN(0) did not panic")
		}
	}()
	games.NewCryptoRNG().IntN(0)
}

func TestWinTierFor(t *testing.T) {
	tests := []struct {
		bet, win int64
		want     api.WinTier
	}{
		{10, 0, api.None},
		{0, 100, api.None},
		{10, 5, api.Normal},
		{10, 99, api.Normal},
		{10, 100, api.Big},
		{10, 249, api.Big},
		{10, 250, api.Mega},
		{10, 500, api.Epic},
		{10, 1_000_000, api.Epic},
	}
	for _, tc := range tests {
		if got := games.WinTierFor(tc.bet, tc.win); got != tc.want {
			t.Errorf("WinTierFor(%d, %d) = %s, want %s", tc.bet, tc.win, got, tc.want)
		}
	}
}

func TestRegistry(t *testing.T) {
	reg := games.NewRegistry()
	b := &gamestest.Fake{ID: "b-game"}
	a := &gamestest.Fake{ID: "a-game"}
	for _, g := range []games.SlotGame{b, a} {
		if err := reg.Register(g); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Register(&gamestest.Fake{ID: "a-game"}); err == nil {
		t.Fatal("duplicate id accepted")
	}
	if err := reg.Register(&gamestest.Fake{}); err == nil {
		t.Fatal("empty id accepted")
	}
	if g, ok := reg.Get("a-game"); !ok || g != a {
		t.Fatal("Get returned the wrong game")
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("Get found a missing game")
	}
	list := reg.List()
	if len(list) != 2 || list[0].Id != "a-game" || list[1].Id != "b-game" {
		t.Fatalf("List not sorted by name: %+v", list)
	}
	if !games.ValidBet(a.Info(), 20) || games.ValidBet(a.Info(), 21) {
		t.Fatal("ValidBet wrong")
	}
}
