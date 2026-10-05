package gamestest

// ScriptedRNG returns pre-set values (each taken modulo n), cycling when it
// runs out. It makes game logic deterministic in tests.
type ScriptedRNG struct {
	Values []int
	i      int
}

func (r *ScriptedRNG) IntN(n int) int {
	if n <= 0 {
		panic("gamestest: IntN called with n <= 0")
	}
	if len(r.Values) == 0 {
		return 0
	}
	v := r.Values[r.i%len(r.Values)]
	r.i++
	return ((v % n) + n) % n
}
