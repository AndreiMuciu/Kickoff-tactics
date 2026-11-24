package game

import "testing"

// What does each half-time order actually change? Same squads, same seeds;
// team 0 switches order for the second half, team 1 stays balanced.
func TestTacticEffects(t *testing.T) {
	won := map[string][3]float64{} // win rate when trailing by one, level, leading by one
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	const n = 4000
	for _, tac := range Tactics {
		var gf, ga, shF, shA, xgF, xgA, poss, win, tk float64
		var sit, sitWin [3]float64 // trailing by one, level, leading by one
		for seed := uint64(1); seed <= n; seed++ {
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
			lus := [2]Lineup{mk(), mk()}
			half := Simulate(seed*29, lus, balanced, 1)
			full := Simulate(seed*29, lus, [2][2]string{{"balanced", tac.ID}, {"balanced", "balanced"}}, 2)
			gf += float64(full.Score[0] - half.Score[0])
			ga += float64(full.Score[1] - half.Score[1])
			shF += float64(full.Stats[0].Shots - half.Stats[0].Shots)
			shA += float64(full.Stats[1].Shots - half.Stats[1].Shots)
			xgF += full.Stats[0].XG - half.Stats[0].XG
			xgA += full.Stats[1].XG - half.Stats[1].XG
			tk += float64(full.Stats[0].Tackles - half.Stats[0].Tackles)
			p0, p1 := float64(full.Stats[0].Poss-half.Stats[0].Poss), float64(full.Stats[1].Poss-half.Stats[1].Poss)
			poss += p0 / (p0 + p1)
			if full.Winner == 0 {
				win++
			}
			if d := half.Score[0] - half.Score[1]; d >= -1 && d <= 1 {
				sit[d+1]++
				if full.Winner == 0 {
					sitWin[d+1]++
				}
			}
		}
		won[tac.ID] = [3]float64{sitWin[0] / sit[0], sitWin[1] / sit[1], sitWin[2] / sit[2]}
		t.Logf("%-15s 2nd half: goals %.2f-%.2f | shots %.2f-%.2f | xG %.2f-%.2f | possession %.0f%% | tackles %.1f | won %.1f%% | won when trailing by 1: %.1f%%, level: %.1f%%, leading by 1: %.1f%%",
			tac.Name, gf/n, ga/n, shF/n, shA/n, xgF/n, xgA/n, 100*poss/n, tk/n, 100*win/n, 100*sitWin[0]/sit[0], 100*sitWin[1]/sit[1], 100*sitWin[2]/sit[2])
	}
	// The orders must be real choices: the right one depends on the score.
	for id, w := range won {
		if id != "all_out" && w[0] >= won["all_out"][0] {
			t.Errorf("%s is at least as good as All-Out Attack when trailing (%.3f vs %.3f)", id, w[0], won["all_out"][0])
		}
		if id != "park_bus" && w[2] >= won["park_bus"][2] {
			t.Errorf("%s is at least as good as Park the Bus when leading (%.3f vs %.3f)", id, w[2], won["park_bus"][2])
		}
		if w[1] > 0.54 || w[1] < 0.45 {
			t.Errorf("%s wins %.3f of level games: one order should not dominate", id, w[1])
		}
	}
	// The sample of games that are one goal apart is small, so the check on the
	// size of the effect is deliberately loose; the ordering above is the strict part.
	if won["all_out"][0] < won["balanced"][0]+0.005 || won["park_bus"][2] < won["balanced"][2]+0.03 {
		t.Errorf("orders barely matter: trailing %.3f vs %.3f, leading %.3f vs %.3f", won["all_out"][0], won["balanced"][0], won["park_bus"][2], won["balanced"][2])
	}
}

// Long Balls and Shoot on Sight are answers to what the other side does: each
// must be the best order against the one it counters.
func TestOrderCounters(t *testing.T) {
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	// Matches where team 0 is `lead` goals ahead at the break.
	games := func(lead int) (out [][2]Lineup, seeds []uint64) {
		for seed := uint64(1); len(out) < 1200; seed++ {
			g := NewRNG(seed)
			mk := func() Lineup {
				lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
				used := map[[2]int]bool{}
				for len(lu.Units) < 5 {
					c := [2]int{g.Intn(3), g.Intn(5)}
					if !used[c] {
						used[c] = true
						lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1], ""})
					}
				}
				return lu
			}
			lus := [2]Lineup{mk(), mk()}
			if h := Simulate(seed*29, lus, balanced, 1); h.Score[0]-h.Score[1] == lead {
				out, seeds = append(out, lus), append(seeds, seed*29)
			}
		}
		return
	}
	rate := func(lus [][2]Lineup, seeds []uint64, mine, theirs string) float64 {
		w := 0
		for i := range lus {
			if Simulate(seeds[i], lus[i], [2][2]string{{"balanced", mine}, {"balanced", theirs}}, 2).Winner == 0 {
				w++
			}
		}
		return float64(w) / float64(len(lus))
	}
	behind, bs := games(-1)
	level, ls := games(0)
	for _, other := range []string{"balanced", "all_out", "counter"} {
		if a, b := rate(behind, bs, "shoot_sight", "park_bus"), rate(behind, bs, other, "park_bus"); a <= b {
			t.Errorf("trailing against Park the Bus: Shoot on Sight wins %.3f, %s %.3f", a, other, b)
		}
	}
	for _, other := range []string{"balanced", "park_bus", "keep_ball"} {
		if a, b := rate(level, ls, "long_balls", "all_out"), rate(level, ls, other, "all_out"); a <= b {
			t.Errorf("level against All-Out Attack: Long Balls wins %.3f, %s %.3f", a, other, b)
		}
	}
}
