package game

import (
	"math"
	"testing"
)

// longestStill returns the longest run of ticks in which the ball barely moved.
func longestStill(r *SimResult) (int, int) {
	best, cur, at := 0, 0, 0
	for i := 1; i < len(r.Frames); i++ {
		a, b := r.Frames[i-1], r.Frames[i]
		if math.Hypot(float64(a[0]-b[0]), float64(a[1]-b[1])) < 2 { // 0.2 pitch units
			cur++
			if cur > best {
				best, at = cur, i
			}
		} else {
			cur = 0
		}
	}
	return best, at
}

func TestNoStalls(t *testing.T) {
	gk := LineupUnit{DefID: AcademyKeeper, Stars: 1}
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	worst, total := 0, 0
	for seed := uint64(1); seed <= 3000; seed++ {
		g := NewRNG(seed)
		mk := func(n int) Lineup {
			lu := Lineup{GK: gk}
			used := map[[2]int]bool{}
			for len(lu.Units) < n {
				c := [2]int{g.Intn(3), g.Intn(5)}
				if used[c] {
					continue
				}
				used[c] = true
				lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1], ""})
			}
			return lu
		}
		// Squads of 0-3 outfield players: a keeper-only side must not freeze the match.
		a, b := mk(g.Intn(4)), mk(g.Intn(4))
		r := Simulate(seed*77, [2]Lineup{a, b}, balanced, 2)
		n, at := longestStill(r)
		if n > worst {
			worst = n
		}
		if n > 60 { // six seconds of a dead ball
			total++
			if total <= 3 {
				f := r.Frames[at]
				t.Logf("seed %d: ball still for %d ticks until tick %d at (%d,%d) owner %d, squads %d v %d, roster %v", seed, n, at, f[0], f[1], f[2], len(a.Units), len(b.Units), r.Roster)
			}
		}
	}
	t.Logf("worst still run: %d ticks; matches with a stall over 6 s: %d / 3000", worst, total)
	if total > 0 {
		t.Errorf("%d matches stalled", total)
	}
}
