package games

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"pantheon-spins/server/internal/api"
)

// Config is a game's JSON config from /game-configs. The schema is documented
// in game-configs/schema/game-config.schema.json. Game-specific settings live
// in Params, which each game decodes into its own struct.
type Config struct {
	// Schema is the optional "$schema" hint for editors; it is ignored.
	Schema      string             `json:"$schema,omitempty"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Theme       string             `json:"theme"`
	Description string             `json:"description"`
	Layout      api.LayoutKind     `json:"layout"`
	Reels       int                `json:"reels"`
	Rows        int                `json:"rows"`
	Heights     []int              `json:"heights,omitempty"`
	Volatility  api.Volatility     `json:"volatility"`
	Tags        []api.GameInfoTags `json:"tags"`
	BetLevels   []int64            `json:"betLevels"`
	DefaultBet  int64              `json:"defaultBet"`
	TargetRTP   struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"targetRtp"`
	Symbols  []SymbolConfig              `json:"symbols"`
	ReelSets map[string][]map[string]int `json:"reelSets"`
	Lines    [][]int                     `json:"lines,omitempty"`
	Params   json.RawMessage             `json:"params,omitempty"`
}

// SymbolConfig is one symbol: pays maps a count (or cluster size) to a bet multiple.
type SymbolConfig struct {
	ID   string             `json:"id"`
	Name string             `json:"name"`
	Kind SymbolKind         `json:"kind"`
	Pays map[string]float64 `json:"pays,omitempty"`
}

// Game is a validated, compiled config ready for a game implementation.
type Game struct {
	Config
	Symbols  *Symbols
	Pays     Paytable
	ReelSets map[string]ReelSet
	// Heights are the reel heights (Rows for every reel unless Heights is set).
	Heights []int
}

var (
	gameIDRe   = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	symbolIDRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
)

// LoadConfig reads and compiles <dir>/<id>.json.
func LoadConfig(dir, id string) (*Game, error) {
	raw, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("game config %s: %w", id, err)
	}
	g, err := ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("game config %s: %w", id, err)
	}
	if g.ID != id {
		return nil, fmt.Errorf("game config %s: file declares id %q", id, g.ID)
	}
	return g, nil
}

// ParseConfig decodes, validates and compiles a config. Unknown fields are errors.
func ParseConfig(raw []byte) (*Game, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return compile(c)
}

func compile(c Config) (*Game, error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !gameIDRe.MatchString(c.ID) {
		fail("id %q must match %s", c.ID, gameIDRe)
	}
	if c.Name == "" {
		fail("name is required")
	}
	if !c.Layout.Valid() {
		fail("layout %q is not valid", c.Layout)
	}
	if !c.Volatility.Valid() {
		fail("volatility %q is not valid", c.Volatility)
	}
	for _, t := range c.Tags {
		if !t.Valid() {
			fail("tag %q is not valid", t)
		}
	}

	// Grid shape.
	heights := c.Heights
	if c.Reels < 1 || c.Reels > 12 || c.Rows < 1 || c.Rows > 12 {
		fail("reels and rows must be 1–12 (got %d×%d)", c.Reels, c.Rows)
	} else if len(heights) == 0 {
		heights = Uniform(c.Reels, c.Rows)
	} else if len(heights) != c.Reels {
		fail("heights has %d entries for %d reels", len(heights), c.Reels)
	} else {
		for _, h := range heights {
			if h < 1 || h > c.Rows {
				fail("heights must be 1–rows (%d), got %d", c.Rows, h)
			}
		}
	}

	// Bets: multiples of UnitsPerBet so every pay converts to whole Coins.
	if len(c.BetLevels) == 0 {
		fail("betLevels is required")
	}
	for i, b := range c.BetLevels {
		if b <= 0 || b%UnitsPerBet != 0 {
			fail("bet level %d must be a positive multiple of %d", b, UnitsPerBet)
		}
		if i > 0 && b <= c.BetLevels[i-1] {
			fail("betLevels must be strictly increasing")
		}
	}
	if !slices.Contains(c.BetLevels, c.DefaultBet) {
		fail("defaultBet %d is not one of betLevels", c.DefaultBet)
	}
	if c.TargetRTP.Min < 0.5 || c.TargetRTP.Max >= 1 || c.TargetRTP.Min > c.TargetRTP.Max {
		fail("targetRtp must satisfy 0.5 ≤ min ≤ max < 1")
	}

	// Symbols and paytable.
	if len(c.Symbols) == 0 || len(c.Symbols) >= int(Empty) {
		fail("symbols must have 1–%d entries", Empty-1)
	}
	defs := make([]SymbolDef, len(c.Symbols))
	pays := make(Paytable, len(c.Symbols))
	seen := map[string]bool{}
	paying := 0
	for i, sc := range c.Symbols {
		if !symbolIDRe.MatchString(sc.ID) {
			fail("symbol id %q must match %s", sc.ID, symbolIDRe)
		}
		if seen[sc.ID] {
			fail("duplicate symbol %q", sc.ID)
		}
		seen[sc.ID] = true
		if !sc.Kind.Valid() {
			fail("symbol %s: kind %q is not valid", sc.ID, sc.Kind)
		}
		name := sc.Name
		if name == "" {
			name = sc.ID
		}
		defs[i] = SymbolDef{ID: sc.ID, Name: name, Kind: sc.Kind}
		if len(sc.Pays) > 0 && sc.Kind == KindSpecial {
			fail("symbol %s: special symbols cannot have pays", sc.ID)
		}
		row, err := payRow(sc.Pays)
		if err != nil {
			fail("symbol %s: %w", sc.ID, err)
		}
		pays[i] = row
		if (sc.Kind == KindLow || sc.Kind == KindHigh) && len(row) > 0 {
			paying++
		}
	}
	if paying == 0 {
		fail("at least one low or high symbol must pay")
	}
	syms := NewSymbols(defs)

	// Reel sets.
	reelSets := map[string]ReelSet{}
	if _, ok := c.ReelSets["base"]; !ok {
		fail(`reelSets must include "base"`)
	}
	for name, reels := range c.ReelSets {
		if len(reels) != c.Reels {
			fail("reelSets.%s has %d reels, want %d", name, len(reels), c.Reels)
			continue
		}
		set := make(ReelSet, len(reels))
		for r, weights := range reels {
			m := map[Symbol]int{}
			for id, w := range weights {
				s, ok := syms.Lookup(id)
				if !ok {
					fail("reelSets.%s[%d]: unknown symbol %q", name, r, id)
					continue
				}
				m[s] = w
			}
			t, err := NewWeightTable(m)
			if err != nil {
				fail("reelSets.%s[%d]: %w", name, r, err)
				continue
			}
			set[r] = t
		}
		reelSets[name] = set
	}

	// Paylines.
	if c.Layout == api.Lines && len(c.Lines) == 0 {
		fail("lines layout requires lines")
	}
	if c.Layout != api.Lines && len(c.Lines) > 0 {
		fail("lines are only valid for the lines layout")
	}
	for i, line := range c.Lines {
		if len(line) != c.Reels {
			fail("line %d has %d entries, want %d", i+1, len(line), c.Reels)
			continue
		}
		for r, row := range line {
			if r < len(heights) && (row < 0 || row >= heights[r]) {
				fail("line %d: row %d is off reel %d", i+1, row, r)
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &Game{Config: c, Symbols: syms, Pays: pays, ReelSets: reelSets, Heights: heights}, nil
}

// payRow converts {"3": 0.5, "5": 2} into a paytable row where each count pays
// the entry for the largest key at or below it.
func payRow(pays map[string]float64) ([]Units, error) {
	if len(pays) == 0 {
		return nil, nil
	}
	byCount := map[int]Units{}
	maxCount := 0
	for k, v := range pays {
		n, err := strconv.Atoi(k)
		if err != nil || n < 1 || n > 100 {
			return nil, fmt.Errorf("pay count %q must be an integer 1–100", k)
		}
		u, err := UnitsFromMultiple(v)
		if err != nil {
			return nil, err
		}
		byCount[n] = u
		maxCount = max(maxCount, n)
	}
	row := make([]Units, maxCount+1)
	var cur Units
	for n := 1; n <= maxCount; n++ {
		if u, ok := byCount[n]; ok {
			cur = u
		}
		row[n] = cur
	}
	return row, nil
}

// Info returns the lobby metadata.
func (g *Game) Info() api.GameInfo {
	tags := g.Tags
	if tags == nil {
		tags = []api.GameInfoTags{}
	}
	return api.GameInfo{
		Id:          g.ID,
		Name:        g.Name,
		Theme:       g.Theme,
		Description: g.Description,
		Layout:      g.Layout,
		Reels:       g.Reels,
		Rows:        g.Rows,
		Volatility:  g.Volatility,
		BetLevels:   g.BetLevels,
		DefaultBet:  g.DefaultBet,
		Tags:        tags,
	}
}

// TargetRange returns the RTP range the game is tuned to (used by the simulator).
func (g *Game) TargetRange() (min, max float64) { return g.TargetRTP.Min, g.TargetRTP.Max }

// DecodeParams strictly decodes the game-specific params into v.
func (g *Game) DecodeParams(v any) error {
	if len(g.Params) == 0 {
		return fmt.Errorf("game %s: params are missing", g.ID)
	}
	dec := json.NewDecoder(bytes.NewReader(g.Params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("game %s: params: %w", g.ID, err)
	}
	return nil
}

// ReelSet returns a named reel set, or an error naming the game.
func (g *Game) ReelSet(name string) (ReelSet, error) {
	rs, ok := g.ReelSets[name]
	if !ok {
		return nil, fmt.Errorf("game %s: missing reel set %q", g.ID, name)
	}
	return rs, nil
}

// Symbol looks up a symbol ID, or returns an error naming the game.
func (g *Game) Symbol(id string) (Symbol, error) {
	s, ok := g.Symbols.Lookup(id)
	if !ok {
		return 0, fmt.Errorf("game %s: missing symbol %q", g.ID, id)
	}
	return s, nil
}
