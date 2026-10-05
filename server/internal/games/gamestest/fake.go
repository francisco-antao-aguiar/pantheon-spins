// Package gamestest provides a minimal SlotGame for testing the wallet, spin
// and bonus plumbing without depending on a real game's maths.
package gamestest

import (
	"encoding/json"
	"fmt"
	"strconv"

	"pantheon-spins/server/internal/api"
	"pantheon-spins/server/internal/games"
)

// Fake pays WinMultiplier × bet with probability 1/WinEvery (never when
// WinEvery is 0), and triggers a 3-pick bonus with probability 1/BonusEvery
// (never when BonusEvery is 0). Each pick pays PickValue × bet.
type Fake struct {
	ID            string
	WinEvery      int
	WinMultiplier int64
	BonusEvery    int
	Picks         int
	PickValue     int64
}

var _ games.SlotGame = (*Fake)(nil)

func (f *Fake) Info() api.GameInfo {
	return api.GameInfo{
		Id: f.ID, Name: "Fake " + f.ID, Theme: "test", Layout: api.Ways,
		Reels: 5, Rows: 3, Volatility: api.Low,
		BetLevels: []int64{10, 20, 50, 100}, DefaultBet: 10, Tags: []api.GameInfoTags{},
	}
}

func (f *Fake) Spin(rng games.RNG, bet int64) (games.SpinOutcome, error) {
	var win int64
	if f.WinEvery > 0 && rng.IntN(f.WinEvery) == 0 {
		win = f.WinMultiplier * bet
	}
	step := api.SpinStep{Grid: api.Grid{{"A"}, {"A"}, {"A"}, {"A"}, {"A"}}, Wins: []api.Win{}, StepWin: win}
	out := games.SpinOutcome{Outcome: api.SpinOutcome{Steps: []api.SpinStep{step}, TotalWin: win}}
	if f.BonusEvery > 0 && rng.IntN(f.BonusEvery) == 0 {
		out.Trigger = &api.BonusTrigger{Kind: api.Pick, Positions: []api.Position{{Reel: 0, Row: 0}}}
	}
	return out, nil
}

type pickState struct {
	Left  int   `json:"left"`
	Bet   int64 `json:"bet"`
	Value int64 `json:"value"`
}

func (f *Fake) StartBonus(_ games.RNG, bet int64, _ api.BonusTrigger) (games.BonusState, error) {
	return f.state(pickState{Left: f.Picks, Bet: bet, Value: f.PickValue})
}

func (f *Fake) state(p pickState) (games.BonusState, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return games.BonusState{}, err
	}
	st := games.BonusState{
		Kind:    api.Pick,
		Done:    p.Left == 0,
		Public:  map[string]any{"picksLeft": p.Left},
		Private: raw,
	}
	if !st.Done {
		st.Actions = []api.BonusActionOption{{Action: "pick", Choices: []string{"0", "1", "2"}}}
	}
	return st, nil
}

func (f *Fake) BonusAction(_ games.RNG, st games.BonusState, a games.Action) (games.BonusState, games.BonusStep, error) {
	var p pickState
	if err := json.Unmarshal(st.Private, &p); err != nil {
		return st, games.BonusStep{}, err
	}
	if st.Done || a.Name != "pick" {
		return st, games.BonusStep{}, games.ErrInvalidAction
	}
	if n, err := strconv.Atoi(a.Choice); err != nil || n < 0 || n > 2 {
		return st, games.BonusStep{}, fmt.Errorf("%w: choice %q", games.ErrInvalidAction, a.Choice)
	}
	p.Left--
	next, err := f.state(p)
	win := p.Value * p.Bet
	return next, games.BonusStep{Win: win, Reveal: map[string]any{"value": win}}, err
}
