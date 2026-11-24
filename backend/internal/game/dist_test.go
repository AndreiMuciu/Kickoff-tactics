package game

import (
	"sort"
	"testing"
)

// An interception must come from a defender standing near the passing line,
// never from someone across the pitch.
func TestInterceptionsAreClose(t *testing.T) {
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	var tackle, cut []float64
	for seed := uint64(1); seed <= 600; seed++ {
		g := NewRNG(seed)
		mk := func() Lineup {
			lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
			used := map[[2]int]bool{}
			for len(lu.Units) < 5 {
				c := [2]int{g.Intn(3), g.Intn(5)}
				if used[c] {
					continue
				}
				used[c] = true
				lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1], ""})
			}
			return lu
		}
		r := Simulate(seed*17, [2]Lineup{mk(), mk()}, balanced, 2)
		at := func(tick, idx int) vec {
			f := r.Frames[max(1, tick)-1]
			return vec{float64(f[3+idx*3]) / 10, float64(f[4+idx*3]) / 10}
		}
		ballAt := func(tick int) vec {
			f := r.Frames[max(1, tick)-1]
			return vec{float64(f[0]) / 10, float64(f[1]) / 10}
		}
		for _, e := range r.Events {
			if e.T < 3 || e.T > len(r.Frames) {
				continue
			}
			switch e.Type {
			case "tackle":
				tackle = append(tackle, dist(at(e.T-1, e.P), at(e.T-1, e.Q)))
			case "intercept":
				// distance between the ball and the man who wins it, 0.3 s before he has it
				cut = append(cut, dist(at(e.T-3, e.P), ballAt(e.T-3)))
			}
		}
	}
	rep := func(name string, v []float64) (float64, float64) {
		sort.Float64s(v)
		q := func(p float64) float64 { return v[int(p*float64(len(v)-1))] }
		t.Logf("%s: n=%d median %.1f, 90%% %.1f, max %.1f units (pitch is 100 long)", name, len(v), q(0.5), q(0.9), q(1))
		return q(0.9), q(1)
	}
	if _, mx := rep("tackle distance", tackle); mx > 6 {
		t.Errorf("a tackle happened from %.1f units", mx)
	}
	if p90, _ := rep("ball to interceptor 0.3 s before he wins it", cut); p90 > 12 {
		t.Errorf("interceptions come from too far: 90%% within %.1f units", p90)
	}
}
