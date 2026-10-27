package sim

import (
	"fmt"
	"math/bits"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// SplitMix64 — the seeding/mixing function
// ---------------------------------------------------------------------------

// splitMix64 is a simple, fast mixer used to derive seeds from a parent seed
// plus a label hash. It is NOT used as the main PRNG — just for seed derivation.
// Based on: http://xoshiro.di.unimi.it/splitmix64.c
func splitMix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// ---------------------------------------------------------------------------
// xoshiro256** — the main PRNG engine
// ---------------------------------------------------------------------------

// xoshiro256ss implements the xoshiro256** algorithm. It has a 256-bit state,
// a period of 2^256 - 1, and passes all known statistical tests. We use it
// instead of Go's math/rand because:
//   1. We need exact reproducibility across Go versions (math/rand's internals
//      can change between releases).
//   2. We need Fork() to derive independent sub-streams, which requires
//      control over the seeding process.
//   3. It's faster than math/rand for our use case (no locking, no interface
//      overhead).
//
// Reference: https://prng.di.unimi.it/xoshiro256starstar.c
type xoshiro256ss struct {
	s [4]uint64
}

// newXoshiro256ss seeds a new xoshiro256** generator from a single uint64 seed.
// Uses SplitMix64 to expand the 64-bit seed into 256-bit state (recommended
// seeding procedure from the xoshiro authors).
func newXoshiro256ss(seed uint64) *xoshiro256ss {
	x := &xoshiro256ss{}
	state := seed
	x.s[0] = splitMix64(&state)
	x.s[1] = splitMix64(&state)
	x.s[2] = splitMix64(&state)
	x.s[3] = splitMix64(&state)
	return x
}

// next produces the next pseudo-random uint64 and advances the state.
func (x *xoshiro256ss) next() uint64 {
	// The "**" scrambler: rotate, multiply, rotate.
	result := bits.RotateLeft64(x.s[1]*5, 7) * 9

	// State transition: xoshiro256 linear engine.
	tt := x.s[1] << 17
	x.s[2] ^= x.s[0]
	x.s[3] ^= x.s[1]
	x.s[1] ^= x.s[2]
	x.s[0] ^= x.s[3]
	x.s[2] ^= tt
	x.s[3] = bits.RotateLeft64(x.s[3], 45)

	return result
}

// ---------------------------------------------------------------------------
// SimRand — the public Rand implementation
// ---------------------------------------------------------------------------

// SimRand implements the types.Rand interface using xoshiro256** as the engine.
// It is the only source of randomness in a simulation.
type SimRand struct {
	rng  *xoshiro256ss
	seed uint64 // stored so we can report it in traces
}

// NewRand creates a new SimRand from the given seed. Typically called once
// per simulation run with the master seed, then Fork() creates named
// sub-streams for different subsystems.
func NewRand(seed uint64) *SimRand {
	return &SimRand{
		rng:  newXoshiro256ss(seed),
		seed: seed,
	}
}

// Seed returns the seed this Rand was created with. Useful for trace metadata.
func (r *SimRand) Seed() uint64 {
	return r.seed
}

// Uint64 returns a uniformly distributed pseudo-random uint64.
func (r *SimRand) Uint64() uint64 {
	return r.rng.next()
}

// Intn returns a pseudo-random int in [0, n). Panics if n <= 0.
// Uses Lemire's nearly divisionless rejection method for uniform distribution
// without modulo bias.
func (r *SimRand) Intn(n int) int {
	if n <= 0 {
		panic(fmt.Sprintf("sim: Intn called with non-positive n=%d", n))
	}
	if n&(n-1) == 0 {
		return int(r.Uint64() & uint64(n-1))
	}
	bound := uint64(n)
	x := r.Uint64()
	hi, lo := bits.Mul64(x, bound)
	if lo < bound {
		threshold := (-bound) % bound
		for lo < threshold {
			x = r.Uint64()
			hi, lo = bits.Mul64(x, bound)
		}
	}
	return int(hi)
}

// Float64 returns a pseudo-random float64 in [0.0, 1.0).
// Uses the top 53 bits for full mantissa precision.
func (r *SimRand) Float64() float64 {
	return float64(r.Uint64()>>11) / float64(1<<53)
}

// Fork creates a new independent Rand derived from this one. The new stream
// is seeded by mixing the parent seed with a hash of the label.
//
// Why not just draw a seed from the parent? Because then the order of Fork
// calls matters. By mixing with the label hash, only the label determines
// the sub-stream — fork order is irrelevant.
func (r *SimRand) Fork(label string) t.Rand {
	h := fnv1a64(label)
	childSeed := r.seed ^ h
	return NewRand(childSeed)
}

// fnv1a64 computes a 64-bit FNV-1a hash of a string. We use our own
// implementation to avoid the allocation of hash.Hash (which needs []byte).
func fnv1a64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// DurationBetween returns a uniformly random Time in [lo, hi].
func DurationBetween(r t.Rand, lo, hi t.Time) t.Time {
	if lo > hi {
		panic(fmt.Sprintf("sim: DurationBetween called with lo=%s > hi=%s", lo, hi))
	}
	if lo == hi {
		return lo
	}
	span := int(hi - lo + 1)
	return lo + t.Time(r.Intn(span))
}

// BoolWithProb returns true with probability p ∈ [0.0, 1.0].
func BoolWithProb(r t.Rand, p float64) bool {
	if p <= 0.0 {
		return false
	}
	if p >= 1.0 {
		return true
	}
	return r.Float64() < p
}

// Compile-time interface check.
var _ t.Rand = (*SimRand)(nil)
