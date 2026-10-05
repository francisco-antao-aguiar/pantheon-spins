package games

import (
	"crypto/rand"
	"encoding/binary"
	"math/bits"
)

// RNG is the only source of randomness game code may use.
type RNG interface {
	// IntN returns a uniform integer in [0, n). It panics if n <= 0.
	IntN(n int) int
}

const rngBufSize = 512

// CryptoRNG draws from crypto/rand through a small buffer. It is not safe for
// concurrent use: create one per request or per simulator goroutine.
type CryptoRNG struct {
	buf [rngBufSize]byte
	pos int
}

func NewCryptoRNG() *CryptoRNG {
	return &CryptoRNG{pos: rngBufSize}
}

func (r *CryptoRNG) uint64() uint64 {
	if r.pos+8 > rngBufSize {
		// crypto/rand.Read never returns an error on supported platforms (Go 1.24+).
		_, _ = rand.Read(r.buf[:])
		r.pos = 0
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v
}

// IntN uses Lemire's multiply-and-reject method, so results are unbiased.
func (r *CryptoRNG) IntN(n int) int {
	if n <= 0 {
		panic("games: IntN called with n <= 0")
	}
	bound := uint64(n)
	hi, lo := bits.Mul64(r.uint64(), bound)
	if lo < bound {
		threshold := -bound % bound
		for lo < threshold {
			hi, lo = bits.Mul64(r.uint64(), bound)
		}
	}
	return int(hi)
}
