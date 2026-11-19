package game

// RNG is a small deterministic generator (splitmix64). The match simulation
// must replay identically from a seed, so it never touches math/rand globals.
// Seeds themselves come from crypto/rand (see run.go).
type RNG struct{ S uint64 }

func NewRNG(seed uint64) *RNG { return &RNG{S: seed} }

func (r *RNG) next() uint64 {
	r.S += 0x9E3779B97F4A7C15
	z := r.S
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Float returns a value in [0, 1).
func (r *RNG) Float() float64 { return float64(r.next()>>11) / float64(1<<53) }

// Intn returns a value in [0, n). n must be positive.
func (r *RNG) Intn(n int) int { return int(r.next() % uint64(n)) }
