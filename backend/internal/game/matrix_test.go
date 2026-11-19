package game

import (
	"fmt"
	"os"
	"testing"
)

// Win chance of the side trailing by one at half-time, for every pair of orders.
func TestOrderMatrix(t *testing.T) {
	// A report, not a check: run it with REPORT=1 go test -run OrderMatrix -v
	if os.Getenv("REPORT") == "" {
		t.Skip("set REPORT=1 to print the order matrix")
	}
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	type game struct {
		seed uint64
		lus  [2]Lineup
	}
	var games []game // matches where team 0 trails by exactly one at the break
	for seed := uint64(1); len(games) < 1500; seed++ {
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
		lus := [2]Lineup{mk(), mk()}
		if h := Simulate(seed*29, lus, balanced, 1); h.Score[1]-h.Score[0] == 1 {
			games = append(games, game{seed * 29, lus})
		}
	}
	head := fmt.Sprintf("%-16s", "trailing \\ leading")
	for _, o := range Tactics {
		head += fmt.Sprintf("%-16s", o.Name)
	}
	t.Log(head)
	for _, mine := range Tactics {
		row := fmt.Sprintf("%-18s", mine.Name)
		for _, theirs := range Tactics {
			w := 0
			for _, g := range games {
				if Simulate(g.seed, g.lus, [2][2]string{{"balanced", mine.ID}, {"balanced", theirs.ID}}, 2).Winner == 0 {
					w++
				}
			}
			row += fmt.Sprintf("%-16s", fmt.Sprintf("%.1f%%", 100*float64(w)/float64(len(games))))
		}
		t.Log(row)
	}
}
