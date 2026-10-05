package play_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/bonus"
	"pantheon-spins/server/internal/db"
	"pantheon-spins/server/internal/games"
	"pantheon-spins/server/internal/games/gamestest"
	"pantheon-spins/server/internal/play"
	"pantheon-spins/server/internal/testutil"
	"pantheon-spins/server/internal/wallet"
)

var env *testutil.Env

func TestMain(m *testing.M) { testutil.Main(m, &env) }

func newService(t *testing.T, gs ...games.SlotGame) *play.Service {
	t.Helper()
	reg := games.NewRegistry()
	for _, g := range gs {
		if err := reg.Register(g); err != nil {
			t.Fatal(err)
		}
	}
	return play.NewService(env.Pool, wallet.NewService(env.Pool), reg)
}

// assertLedgerConsistent checks the balance against the spin log and the ledger.
func assertLedgerConsistent(t *testing.T, userID uuid.UUID, start int64) int64 {
	t.Helper()
	ctx := context.Background()
	q := db.New(env.Pool)
	bal, err := q.GetBalance(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	sums, err := q.SumSpins(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	bonusWins, err := q.SumBonusWins(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if want := start - sums.Bets + sums.Wins + bonusWins; bal != want {
		t.Fatalf("balance %d, want start %d - bets %d + wins %d + bonus %d = %d",
			bal, start, sums.Bets, sums.Wins, bonusWins, want)
	}
	txs, err := q.ListTransactions(ctx, db.ListTransactionsParams{UserID: userID, Lim: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) > 0 && txs[0].BalanceAfter != bal {
		t.Fatalf("latest ledger balance_after %d, wallet balance %d", txs[0].BalanceAfter, bal)
	}
	return bal
}

func TestParallelSpinsKeepBalanceConsistent(t *testing.T) {
	testutil.SkipIfShort(t)
	game := &gamestest.Fake{ID: "fake-par", WinEvery: 3, WinMultiplier: 2}
	svc := newService(t, game)
	const start, bet, n = 100_000, 10, 300
	user := testutil.NewUser(t, env.Pool, start)

	var wg sync.WaitGroup
	var failed atomic.Int64
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Spin(context.Background(), user, game.ID, bet); err != nil {
				failed.Add(1)
				t.Errorf("spin: %v", err)
			}
		}()
	}
	wg.Wait()
	if failed.Load() > 0 {
		t.FailNow()
	}

	assertLedgerConsistent(t, user, start)
	sums, _ := db.New(env.Pool).SumSpins(context.Background(), user)
	if sums.Spins != n {
		t.Fatalf("logged %d spins, want %d", sums.Spins, n)
	}
}

func TestParallelSpinsCannotOverspend(t *testing.T) {
	testutil.SkipIfShort(t)
	game := &gamestest.Fake{ID: "fake-over"} // never wins
	svc := newService(t, game)
	const start, bet, n = 55, 10, 100
	user := testutil.NewUser(t, env.Pool, start)

	var wg sync.WaitGroup
	var ok, broke atomic.Int64
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Spin(context.Background(), user, game.ID, bet)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, wallet.ErrInsufficientFunds):
				broke.Add(1)
			default:
				t.Errorf("spin: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok.Load() != start/bet || broke.Load() != n-start/bet {
		t.Fatalf("%d spins succeeded and %d were rejected; want %d and %d", ok.Load(), broke.Load(), start/bet, n-start/bet)
	}
	if bal := assertLedgerConsistent(t, user, start); bal != start%bet {
		t.Fatalf("final balance %d, want %d", bal, start%bet)
	}
}

func TestSpinValidation(t *testing.T) {
	testutil.SkipIfShort(t)
	game := &gamestest.Fake{ID: "fake-valid"}
	svc := newService(t, game)
	user := testutil.NewUser(t, env.Pool, 1000)

	tests := []struct {
		name   string
		gameID string
		bet    int64
		want   error
	}{
		{"unknown game", "nope", 10, play.ErrUnknownGame},
		{"bet not offered", game.ID, 15, play.ErrInvalidBet},
		{"zero bet", game.ID, 0, play.ErrInvalidBet},
		{"negative bet", game.ID, -10, play.ErrInvalidBet},
		{"above top bet level", game.ID, 10_000, play.ErrInvalidBet},
		{"valid", game.ID, 100, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Spin(context.Background(), user, tc.gameID, tc.bet)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestInsufficientFundsChangesNothing(t *testing.T) {
	testutil.SkipIfShort(t)
	game := &gamestest.Fake{ID: "fake-broke"}
	svc := newService(t, game)
	user := testutil.NewUser(t, env.Pool, 5)

	if _, err := svc.Spin(context.Background(), user, game.ID, 10); !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("got %v, want ErrInsufficientFunds", err)
	}
	if bal := assertLedgerConsistent(t, user, 5); bal != 5 {
		t.Fatalf("balance %d, want 5", bal)
	}
}

func TestBonusLifecycle(t *testing.T) {
	testutil.SkipIfShort(t)
	ctx := context.Background()
	game := &gamestest.Fake{ID: "fake-bonus", BonusEvery: 1, Picks: 3, PickValue: 5}
	svc := newService(t, game)
	const start, bet = 1000, 10
	user := testutil.NewUser(t, env.Pool, start)

	res, err := svc.Spin(ctx, user, game.ID, bet)
	if err != nil {
		t.Fatal(err)
	}
	if res.BonusTrigger.Kind != api.Pick || res.Bonus.Status != api.Active || res.Bonus.Step != 0 {
		t.Fatalf("spin did not start the bonus: trigger=%+v bonus=%+v", res.BonusTrigger, res.Bonus)
	}
	if res.Bonus.Data["private"] != nil {
		t.Fatal("private state leaked to the client")
	}

	// A new spin is refused while the bonus is unresolved.
	if _, err := svc.Spin(ctx, user, game.ID, bet); !errors.Is(err, play.ErrBonusInProgress) {
		t.Fatalf("second spin: got %v, want ErrBonusInProgress", err)
	}

	// Resuming (e.g. after a refresh) returns the same bonus.
	resumed, err := svc.ActiveBonus(ctx, user, game.ID)
	if err != nil || resumed.BonusId != res.Bonus.BonusId {
		t.Fatalf("resume: %v, %+v", err, resumed)
	}

	steps := []struct {
		name    string
		step    int
		action  games.Action
		wantErr error
	}{
		{"stale step", 5, games.Action{Name: "pick", Choice: "0"}, play.ErrStaleStep},
		{"unknown action", 0, games.Action{Name: "dance"}, games.ErrInvalidAction},
		{"bad choice", 0, games.Action{Name: "pick", Choice: "9"}, games.ErrInvalidAction},
		{"pick 1", 0, games.Action{Name: "pick", Choice: "0"}, nil},
		{"replayed pick 1", 0, games.Action{Name: "pick", Choice: "0"}, play.ErrStaleStep},
		{"pick 2", 1, games.Action{Name: "pick", Choice: "1"}, nil},
		{"pick 3", 2, games.Action{Name: "pick", Choice: "2"}, nil},
	}
	var last api.BonusStepResult
	for _, s := range steps {
		r, err := svc.BonusAction(ctx, user, game.ID, s.step, s.action)
		if !errors.Is(err, s.wantErr) {
			t.Fatalf("%s: got %v, want %v", s.name, err, s.wantErr)
		}
		if err == nil {
			if r.StepWin != 5*bet {
				t.Fatalf("%s: step win %d, want %d", s.name, r.StepWin, 5*bet)
			}
			last = r
		}
	}
	if last.State.Status != api.Completed || last.State.TotalWin != 15*bet || len(last.State.Actions) != 0 {
		t.Fatalf("final state: %+v", last.State)
	}
	if _, err := svc.ActiveBonus(ctx, user, game.ID); !errors.Is(err, bonus.ErrNoActiveBonus) {
		t.Fatalf("after completion: got %v, want ErrNoActiveBonus", err)
	}
	if _, err := svc.BonusAction(ctx, user, game.ID, 3, games.Action{Name: "pick", Choice: "0"}); !errors.Is(err, bonus.ErrNoActiveBonus) {
		t.Fatalf("action after completion: got %v, want ErrNoActiveBonus", err)
	}
	if bal := assertLedgerConsistent(t, user, start); bal != start-bet+15*bet || bal != last.Balance {
		t.Fatalf("balance %d (reported %d), want %d", bal, last.Balance, start-bet+15*bet)
	}

	// The bonus is resolved, so spinning works again.
	if _, err := svc.Spin(ctx, user, game.ID, bet); err != nil {
		t.Fatalf("spin after bonus: %v", err)
	}
}

func TestConcurrentBonusActionsApplyOnce(t *testing.T) {
	testutil.SkipIfShort(t)
	ctx := context.Background()
	game := &gamestest.Fake{ID: "fake-race", BonusEvery: 1, Picks: 3, PickValue: 5}
	svc := newService(t, game)
	user := testutil.NewUser(t, env.Pool, 1000)
	if _, err := svc.Spin(ctx, user, game.ID, 10); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var ok, stale atomic.Int64
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.BonusAction(ctx, user, game.ID, 0, games.Action{Name: "pick", Choice: "1"})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, play.ErrStaleStep):
				stale.Add(1)
			default:
				t.Errorf("bonus action: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || stale.Load() != 19 {
		t.Fatalf("%d applied, %d stale; want 1 and 19", ok.Load(), stale.Load())
	}
	assertLedgerConsistent(t, user, 1000)
}

func TestForceBonus(t *testing.T) {
	testutil.SkipIfShort(t)
	ctx := context.Background()
	game := &gamestest.Fake{ID: "fake-force", BonusEvery: 50, Picks: 1, PickValue: 1}
	svc := newService(t, game)
	user := testutil.NewUser(t, env.Pool, 1000)

	res, err := svc.ForceBonus(ctx, user, game.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Bonus.Status != api.Active || res.Balance != 990 {
		t.Fatalf("forced spin: bonus %+v balance %d", res.Bonus, res.Balance)
	}
	// It is a normal paid spin: one bet charged, one spin logged.
	assertLedgerConsistent(t, user, 1000)
	if _, err := svc.ForceBonus(ctx, user, game.ID, 10); !errors.Is(err, play.ErrBonusInProgress) {
		t.Fatalf("second force while a bonus is active: %v", err)
	}

	never := &gamestest.Fake{ID: "fake-never"}
	svc = newService(t, never)
	if _, err := svc.ForceBonus(ctx, user, never.ID, 10); err == nil || !strings.Contains(err.Error(), "no bonus") {
		t.Fatalf("game without a bonus: %v", err)
	}
	assertLedgerConsistent(t, user, 1000)
}
