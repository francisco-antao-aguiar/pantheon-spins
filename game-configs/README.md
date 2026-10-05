# Game configs

One JSON file per slot game (`<gameId>.json`): symbols, per-reel weights,
paytable, bonus parameters and target RTP. The server's game packages and the
web renderers both read these files, and the RTP simulator
(`go run ./cmd/simulate -game <gameId>`) tunes them.

The schema and the first configs arrive with the game engine (build step 2).
