# Game configs

One JSON file per slot game, named `<gameId>.json`. Each file holds the
symbols, the per-reel weights, the paytable, the bonus parameters and the
target RTP. The server's game packages and the web renderers both read these
files. The RTP simulator measures them:

```bash
cd server && go run ./cmd/simulate -game <gameId> -spins 10000000
# or, without a local Go install:
docker compose run --rm gotools go run ./cmd/simulate -game <gameId>
```

The format is described by [`schema/game-config.schema.json`](schema/game-config.schema.json).
Add `"$schema": "./schema/game-config.schema.json"` to a config for editor
validation. The server checks the same rules, and a few more, when it loads.

## Conventions

- **Pays are bet multiples in steps of 0.05**, e.g. `"pays": {"3": 0.25, "4": 1, "5": 5}`.
  Bet levels are multiples of 20 Coins, so every win is a whole number of Coins at every bet.
- **Counts** are reels in a row for ways and lines games, and cluster size for
  cluster games. A count pays the entry for the largest key at or below it, so
  `{"5": 0.5, "8": 1}` pays 0.5× for clusters of 5–7 and 1× for 8 or more.
- **Reel weights:** each cell is drawn independently from its reel's weights.
  Named sets let features switch weights, e.g. `base` and `free`.
- **Kinds:** `low` and `high` pay normally. `wild` substitutes for them.
  `scatter` pays or triggers anywhere. `special` is anything game-specific
  (orbs, pearls, lanterns…).
- **Game-specific maths** goes under `params`. Each game decodes `params` strictly
  into its own struct, so a typo fails at startup rather than silently.
