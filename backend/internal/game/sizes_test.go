package game

import (
	"math"
	"os"
	"testing"
)

// A report of how matches look at every squad size the game allows (1 to 7
// outfield players) plus a full eleven for reference.
// Run it with: REPORT=1 go test -run SquadSizes -v ./internal/game/
func TestSquadSizes(t *testing.T) {
	if os.Getenv("REPORT") == "" {
		t.Skip("set REPORT=1 to print the squad-size report")
	}
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	const n = 3000
	t.Logf("%-4s %-6s %-6s %-6s %-7s %-7s %-8s %-7s %-6s %-9s %-8s %-7s %-6s", "size", "goals", "shots", "xG", "passes", "duels", "tackles", "fouls", "yell.", "sent off", "penalty", "pens %", "stall")
	for _, size := range sizesToReport() {
		ev := map[string]int{}
		var goals, shots, passes, tackles, shoot, stalled int
		var xg float64
		worst := 0
		for seed := uint64(1); seed <= n; seed++ {
			g := NewRNG(seed + uint64(size)*100000)
			mk := func() Lineup {
				lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
				used := map[[2]int]bool{}
				taken := map[string]bool{}
				for len(lu.Units) < size {
					c := [2]int{g.Intn(3), g.Intn(5)}
					id := ids[g.Intn(len(ids))]
					if used[c] || taken[id] {
						continue
					}
					used[c], taken[id] = true, true
					lu.Units = append(lu.Units, LineupUnit{id, 1, c[0], c[1]})
				}
				return lu
			}
			r := Simulate(seed*53+uint64(size), [2]Lineup{mk(), mk()}, balanced, 2)
			for _, e := range r.Events {
				ev[e.Type]++
			}
			goals += r.Score[0] + r.Score[1]
			shots += r.Stats[0].Shots + r.Stats[1].Shots
			passes += r.Stats[0].Passes + r.Stats[1].Passes
			tackles += r.Stats[0].Tackles + r.Stats[1].Tackles
			xg += r.Stats[0].XG + r.Stats[1].XG
			if len(r.Pens) > 0 {
				shoot++
			}
			// longest spell with the ball standing still during the 90 minutes
			cur, best := 0, 0
			for i := 1; i < 2*HalfTicks; i++ {
				a, b := r.Frames[i-1], r.Frames[i]
				if math.Hypot(float64(a[0]-b[0]), float64(a[1]-b[1])) < 2 {
					cur++
					best = max(best, cur)
				} else {
					cur = 0
				}
			}
			worst = max(worst, best)
			if best > 60 {
				stalled++
			}
		}
		f := func(v int) float64 { return float64(v) / n }
		t.Logf("%-4d %-6.2f %-6.1f %-6.2f %-7.1f %-7.1f %-8.1f %-7.2f %-6.2f %-9.3f %-8.2f %-7.0f %d (max %.1fs)",
			size, f(goals), f(shots), xg/n, f(passes), f(ev["dribble"]+ev["tackle"]), f(tackles), f(ev["foul"]), f(ev["yellow"]), f(ev["red"]+ev["red2"]), f(ev["penalty"]), 100*f(shoot), stalled, float64(worst)/10)
	}
}

// sizesToReport is every squad size by default, or the list in SIZES (e.g. SIZES=5,7).
func sizesToReport() []int {
	out := []int{}
	n := 0
	for _, c := range os.Getenv("SIZES") + "," {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else if n > 0 {
			out = append(out, n)
			n = 0
		}
	}
	if len(out) == 0 {
		return []int{1, 2, 3, 4, 5, 6, 7, 10}
	}
	return out
}
