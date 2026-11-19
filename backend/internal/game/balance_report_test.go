package game

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// Balance reports. They print numbers and check nothing: run them with
//
//	REPORT=1 go test -run 'Balance' -v ./internal/game/
//
// Every comparison is a mirror or a like-for-like one, so a number far from
// 50% means that one thing is too strong or too weak, not that the teams differ.

var naturalLine = map[string]int{"defender": 0, "anchor": 1, "playmaker": 1, "winger": 2, "striker": 2}

func outfieldPool(keep func(PlayerDef) bool) []string {
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" && (keep == nil || keep(p)) {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// place puts players on their natural line, in the first free lane.
func place(ids []string, stars int) Lineup {
	lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
	used := map[int]int{}
	for _, id := range ids {
		line := naturalLine[PlayerByID[id].Role]
		for used[line] >= 5 {
			line = (line + 1) % 3
		}
		lane := []int{2, 1, 3, 0, 4}[used[line]]
		used[line]++
		lu.Units = append(lu.Units, LineupUnit{DefID: id, Stars: stars, Line: line, Lane: lane})
	}
	return lu
}

// shape lines seven players up 2-3-2 the way a manager would: the most
// defensive at the back, the most attacking up front, whatever their style.
func shape(ids []string) Lineup {
	order := map[string]int{"defender": 0, "anchor": 1, "playmaker": 2, "winger": 3, "striker": 4}
	sorted := append([]string{}, ids...)
	sort.SliceStable(sorted, func(a, b int) bool { return order[PlayerByID[sorted[a]].Role] < order[PlayerByID[sorted[b]].Role] })
	lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
	cells := [][2]int{{0, 1}, {0, 3}, {1, 2}, {1, 0}, {1, 4}, {2, 1}, {2, 3}}
	for i, id := range sorted {
		c := cells[i%len(cells)]
		lu.Units = append(lu.Units, LineupUnit{DefID: id, Stars: 1, Line: c[0], Lane: c[1]})
	}
	return lu
}

func pick(g *RNG, pool []string, n int, taken map[string]bool) []string {
	var out []string
	for tries := 0; len(out) < n && tries < 10000; tries++ {
		id := pool[g.Intn(len(pool))]
		if !taken[id] {
			taken[id] = true
			out = append(out, id)
		}
	}
	return out
}

// mirror plays the same line-up against itself, one side without `skip`, and
// returns how often the side that has it wins. Sides alternate.
func mirror(lu Lineup, skip string, seeds int, salt uint64) (wins, n int) {
	for k := 0; k < seeds; k++ {
		without := [2]string{"", skip}
		if k%2 == 1 {
			without = [2]string{skip, ""}
		}
		r := simulate(salt*7919+uint64(k)*31+1, [2]Lineup{lu, lu}, balanced, 2, without)
		if r.Winner == k%2 {
			wins++
		}
		n++
	}
	return
}

func skipUnlessReport(t *testing.T) {
	t.Helper()
	if os.Getenv("REPORT") == "" {
		t.Skip("set REPORT=1 to print the balance reports")
	}
}

// What each combination step is worth: same eight players, with and without it.
func TestBalanceCombinations(t *testing.T) {
	skipUnlessReport(t)
	for _, s := range Synergies {
		row := fmt.Sprintf("%-18s", s.Name)
		for _, need := range s.Tiers {
			wins, n := 0, 0
			for l := 0; l < 80; l++ {
				g := NewRNG(uint64(l)*977 + uint64(need)*13 + 5)
				taken := map[string]bool{}
				var ids []string
				if s.Kind == "style" {
					ids = pick(g, outfieldPool(func(p PlayerDef) bool { return p.Style == s.ID }), need, taken)
					// Fill up with one player from each of the other styles: no second style is active.
					var others []string
					for _, o := range Synergies {
						if o.Kind == "style" && o.ID != s.ID {
							others = append(others, o.ID)
						}
					}
					for i := 0; len(ids) < 7; i++ {
						st := others[(i+l)%len(others)]
						ids = append(ids, pick(g, outfieldPool(func(p PlayerDef) bool { return p.Style == st }), 1, taken)...)
					}
				} else {
					ids = pick(g, outfieldPool(func(p PlayerDef) bool { return p.Role == s.ID }), need, taken)
					ids = append(ids, pick(g, outfieldPool(func(p PlayerDef) bool { return p.Role != s.ID }), 7-need, taken)...)
				}
				w, m := mirror(place(ids, 1), s.ID, 50, uint64(l)+uint64(need)*1000)
				wins, n = wins+w, n+m
			}
			row += fmt.Sprintf("  at %d: %5.1f%%", need, 100*float64(wins)/float64(n))
		}
		t.Log(row)
	}
}

// Full style teams against each other: six of a style plus one guest.
func TestBalanceStyleTeams(t *testing.T) {
	skipUnlessReport(t)
	var styles []SynergyDef
	for _, s := range Synergies {
		if s.Kind == "style" {
			styles = append(styles, s)
		}
	}
	team := func(g *RNG, style string) Lineup {
		taken := map[string]bool{}
		ids := pick(g, outfieldPool(func(p PlayerDef) bool { return p.Style == style }), 6, taken)
		ids = append(ids, pick(g, outfieldPool(func(p PlayerDef) bool { return p.Style != style }), 1, taken)...)
		return shape(ids)
	}
	total := make([]float64, len(styles))
	gf, ga := make([]float64, len(styles)), make([]float64, len(styles))
	head := fmt.Sprintf("%-16s", "")
	for _, s := range styles {
		head += fmt.Sprintf("%-9s", s.ID)
	}
	t.Log(head + "average")
	for i, a := range styles {
		row := fmt.Sprintf("%-16s", a.Name)
		for j, b := range styles {
			if i == j {
				row += fmt.Sprintf("%-9s", "-")
				continue
			}
			wins := 0
			const n = 1200
			for k := 0; k < n; k++ {
				g := NewRNG(uint64(i*100+j)*7 + uint64(k)*3 + 11)
				x, y := team(g, a.ID), team(g, b.ID)
				lus, me := [2]Lineup{x, y}, k%2
				if me == 1 {
					lus = [2]Lineup{y, x}
				}
				r := Simulate(uint64(k)*17+3, lus, balanced, 2)
				if r.Winner == me {
					wins++
				}
				gf[i] += float64(r.Score[me]) / n / float64(len(styles)-1)
				ga[i] += float64(r.Score[1-me]) / n / float64(len(styles)-1)
			}
			p := 100 * float64(wins) / n
			total[i] += p / float64(len(styles)-1)
			row += fmt.Sprintf("%-9s", fmt.Sprintf("%.0f%%", p))
		}
		t.Log(row + fmt.Sprintf("%.1f%%  goals %.2f-%.2f", total[i], gf[i], ga[i]))
	}
}

// Every player in a team of his own price against random teams of that price.
func TestBalancePlayers(t *testing.T) {
	skipUnlessReport(t)
	for cost := 1; cost <= 5; cost++ {
		pool := outfieldPool(func(p PlayerDef) bool { return p.Cost == cost })
		type row struct {
			id  string
			win float64
		}
		var rows []row
		for pi, id := range pool {
			wins := 0
			const n = 1600
			for k := 0; k < n; k++ {
				g := NewRNG(uint64(pi)*1009 + uint64(k)*7 + uint64(cost))
				mine := map[string]bool{id: true}
				a := place(append([]string{id}, pick(g, pool, 4, mine)...), 1)
				b := place(pick(g, pool, 5, map[string]bool{id: true}), 1)
				if k%2 == 0 {
					if Simulate(uint64(k)*13+7, [2]Lineup{a, b}, balanced, 2).Winner == 0 {
						wins++
					}
				} else if Simulate(uint64(k)*13+7, [2]Lineup{b, a}, balanced, 2).Winner == 1 {
					wins++
				}
			}
			rows = append(rows, row{id, 100 * float64(wins) / n})
		}
		sort.Slice(rows, func(a, b int) bool { return rows[a].win > rows[b].win })
		var parts []string
		for _, r := range rows {
			d := PlayerByID[r.id]
			parts = append(parts, fmt.Sprintf("%s(%s) %.0f", d.Name, d.Role[:2], r.win))
		}
		t.Logf("cost %d: %s", cost, strings.Join(parts, " | "))
	}
}

// Every keeper against the academy keeper, with the same outfield players in front.
func TestBalanceKeepers(t *testing.T) {
	skipUnlessReport(t)
	pool := outfieldPool(func(p PlayerDef) bool { return p.Cost == 3 })
	var parts []string
	for _, p := range Players {
		if p.Role != "gk" || p.ID == AcademyKeeper {
			continue
		}
		wins := 0
		const n = 2000
		for k := 0; k < n; k++ {
			g := NewRNG(uint64(k)*5 + 3)
			ids := pick(g, pool, 5, map[string]bool{})
			a, b := place(ids, 1), place(ids, 1)
			a.GK = LineupUnit{DefID: p.ID, Stars: 1}
			// Styles are left out on both sides: this is about the gloves.
			if k%2 == 0 {
				if simulate(uint64(k)*19+1, [2]Lineup{a, b}, balanced, 2, [2]string{p.Style, ""}).Winner == 0 {
					wins++
				}
			} else if simulate(uint64(k)*19+1, [2]Lineup{b, a}, balanced, 2, [2]string{"", p.Style}).Winner == 1 {
				wins++
			}
		}
		parts = append(parts, fmt.Sprintf("%s(%d) %.0f%%", p.Name, p.Cost, 100*float64(wins)/n))
	}
	t.Log(strings.Join(parts, " | "))
}

// What a price step and a star are worth.
func TestBalanceCostAndStars(t *testing.T) {
	skipUnlessReport(t)
	const n = 3000
	for cost := 2; cost <= 5; cost++ {
		hi := outfieldPool(func(p PlayerDef) bool { return p.Cost == cost })
		lo := outfieldPool(func(p PlayerDef) bool { return p.Cost == cost-1 })
		wins := 0
		for k := 0; k < n; k++ {
			g := NewRNG(uint64(k)*3 + uint64(cost))
			a, b := place(pick(g, hi, 5, map[string]bool{}), 1), place(pick(g, lo, 5, map[string]bool{}), 1)
			if k%2 == 0 {
				if Simulate(uint64(k)*23+5, [2]Lineup{a, b}, balanced, 2).Winner == 0 {
					wins++
				}
			} else if Simulate(uint64(k)*23+5, [2]Lineup{b, a}, balanced, 2).Winner == 1 {
				wins++
			}
		}
		t.Logf("five cost-%d players beat five cost-%d players in %.1f%%", cost, cost-1, 100*float64(wins)/n)
	}
	all := outfieldPool(nil)
	for _, c := range [][2]int{{2, 1}, {3, 2}, {3, 1}} {
		wins := 0
		for k := 0; k < n; k++ {
			g := NewRNG(uint64(k)*3 + 77)
			ids := pick(g, all, 5, map[string]bool{})
			a, b := place(ids, c[0]), place(ids, c[1])
			if k%2 == 0 {
				if Simulate(uint64(k)*29+5, [2]Lineup{a, b}, balanced, 2).Winner == 0 {
					wins++
				}
			} else if Simulate(uint64(k)*29+5, [2]Lineup{b, a}, balanced, 2).Winner == 1 {
				wins++
			}
		}
		t.Logf("the same five players at %d★ beat themselves at %d★ in %.1f%%", c[0], c[1], 100*float64(wins)/n)
	}
	// Is a cheap squad with stars worth an expensive one without?
	for _, c := range [][4]int{{1, 2, 3, 1}, {1, 3, 4, 1}, {2, 2, 4, 1}, {3, 2, 5, 1}} {
		a0 := outfieldPool(func(p PlayerDef) bool { return p.Cost == c[0] })
		b0 := outfieldPool(func(p PlayerDef) bool { return p.Cost == c[2] })
		wins := 0
		for k := 0; k < n; k++ {
			g := NewRNG(uint64(k)*3 + 99)
			a, b := place(pick(g, a0, 5, map[string]bool{}), c[1]), place(pick(g, b0, 5, map[string]bool{}), c[3])
			if k%2 == 0 {
				if Simulate(uint64(k)*31+5, [2]Lineup{a, b}, balanced, 2).Winner == 0 {
					wins++
				}
			} else if Simulate(uint64(k)*31+5, [2]Lineup{b, a}, balanced, 2).Winner == 1 {
				wins++
			}
		}
		t.Logf("five cost-%d players at %d★ beat five cost-%d players at %d★ in %.1f%%", c[0], c[1], c[2], c[3], 100*float64(wins)/n)
	}
}

// How a solo season goes for a manager who plays like the lobby bots do:
// buys the dearest player he can afford, completes merges, levels up when rich.
func TestBalanceSoloSeason(t *testing.T) {
	skipUnlessReport(t)
	results, reached := map[string]int{}, [MaxRounds + 1]int{}
	const n = 400
	for k := 0; k < n; k++ {
		r, err := NewRun(fmt.Sprintf("solo-%d", k))
		if err != nil {
			t.Fatal(err)
		}
		g := NewRNG(uint64(k)*37 + 1)
		for r.Phase == "shop" {
			reached[r.Round]++
			r.BotManage(g)
			if _, err := r.StartMatch(); err != nil {
				t.Fatal(err)
			}
			if _, _, err := r.FinishMatch("balanced"); err != nil {
				t.Fatal(err)
			}
		}
		results[r.Result]++
	}
	t.Logf("results of %d seasons: %v", n, results)
	t.Logf("seasons that reached each fixture: %v", reached[1:])
}
