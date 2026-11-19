package game

import "testing"

// duelRates plays many matches with random squads and reports, per position,
// how often a player who is challenged with the ball keeps it.
func duelRates(t *testing.T) map[string]float64 {
	t.Helper()
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	kept, lost := map[string]int{}, map[string]int{}
	for seed := uint64(1); seed <= 1500; seed++ {
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
				lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1]})
			}
			return lu
		}
		r := Simulate(seed*13, [2]Lineup{mk(), mk()}, balanced, 2)
		for _, e := range r.Events {
			switch e.Type {
			case "dribble":
				kept[r.Roster[e.P].Role]++
			case "tackle":
				lost[r.Roster[e.Q].Role]++
			}
		}
	}
	out := map[string]float64{}
	for _, role := range []string{"striker", "winger", "playmaker", "anchor", "defender"} {
		n := kept[role] + lost[role]
		out[role] = float64(kept[role]) / float64(max(1, n))
		t.Logf("%-10s keeps the ball in %.0f%% of %d duels (%.1f duels per match)", role, 100*out[role], n, float64(n)/1500)
	}
	return out
}

// Dribblers should beat a challenge far more often than defenders do.
func TestDefendersDoNotDribble(t *testing.T) {
	r := duelRates(t)
	if r["defender"] > 0.42 {
		t.Errorf("defenders keep the ball in %.0f%% of duels, want at most 42%%", 100*r["defender"])
	}
	if r["winger"] < r["defender"]+0.15 {
		t.Errorf("wingers (%.0f%%) should dribble clearly better than defenders (%.0f%%)", 100*r["winger"], 100*r["defender"])
	}
}
