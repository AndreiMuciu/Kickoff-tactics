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
	const n = 1500
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
	results, reached, won := map[string]int{}, [MaxRounds + CupGroupDays + 4]int{}, [MaxRounds + CupGroupDays + 4]int{}
	cups := map[string]map[string]int{} // cup -> outcome -> seasons
	positions, finished, champPts, myPts := [LeagueTeams + 1]int{}, 0, 0, 0
	const n = 1500
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
			round := r.Round
			_, h, err := r.FinishMatch("balanced")
			if err != nil {
				t.Fatal(err)
			}
			if h.Won {
				won[round]++
			}
		}
		results[r.Result]++
		if r.Cup != nil {
			if cups[r.Cup.ID] == nil {
				cups[r.Cup.ID] = map[string]int{}
			}
			cups[r.Cup.ID][r.Cup.Out]++
		}
		if r.Result != "sacked" {
			finished++
			positions[r.League.Position()]++
			champPts += r.League.Table()[0].Pts
			myPts += r.League.view().Points
		}
	}
	t.Logf("results of %d seasons: %v", n, results)
	t.Logf("seasons that reached each fixture: %v", reached[1:])
	rates := make([]string, 0, MaxRounds)
	for i := 1; i < len(reached); i++ {
		rates = append(rates, fmt.Sprintf("%d%%", 100*won[i]/max(reached[i], 1)))
	}
	t.Logf("win rate per fixture: %v", rates)
	t.Logf("final positions of finished seasons (1..10): %v", positions[1:])
	t.Logf("average champion points: %.1f, your average points: %.1f", float64(champPts)/float64(max(finished, 1)), float64(myPts)/float64(max(finished, 1)))
	for _, c := range Cups {
		o, n := cups[c.ID], 0
		for _, v := range o {
			n += v
		}
		line := ""
		for _, out := range []string{"group", "quarter", "semi", "final", "winner", "promoted", "stayed", "sacked"} {
			if o[out] > 0 {
				line += fmt.Sprintf(" %s %d%%", out, 100*o[out]/n)
			}
		}
		t.Logf("%s: %d entries:%s", c.Name, n, line)
	}
}

// What each augment is worth in a lobby of eight bot clubs: every club is
// handed a different augment at the first augment round, later offers are
// taken at random, and the average final place is reported per augment
// (4.5 is neutral, lower is better). Bots ignore styles, so crests measure
// only the extra player's worth in whatever style the bot happened to sign.
func TestBalanceAugments(t *testing.T) {
	skipUnlessReport(t)
	const lobbies = 900
	sum, cnt := map[string]float64{}, map[string]int{}
	type res struct{ place map[string]int }
	out := make(chan res)
	for k := 0; k < lobbies; k++ {
		go func(k int) {
			g := NewRNG(uint64(k)*7919 + 3)
			ids := make([]string, len(Augments))
			for i, a := range Augments {
				ids[i] = a.ID
			}
			for i := len(ids) - 1; i > 0; i-- {
				j := g.Intn(i + 1)
				ids[i], ids[j] = ids[j], ids[i]
			}
			clubs := make([]*Run, 8)
			for i := range clubs {
				clubs[i], _ = NewVersusRun(fmt.Sprintf("l%d-%d", k, i))
			}
			alive := []int{0, 1, 2, 3, 4, 5, 6, 7}
			placed := map[string]int{}
			for round := 1; len(alive) > 1 && round < 60; round++ {
				for _, i := range alive {
					c := clubs[i]
					if len(c.AugmentOffer) > 0 {
						if c.Round == AugmentRounds[0] {
							c.AugmentOffer[0] = ids[i]
							_ = c.chooseAugment(0)
						} else {
							_ = c.chooseAugment(g.Intn(len(c.AugmentOffer)))
						}
					}
					c.BotManage(g)
					c.AutoField()
				}
				order := append([]int(nil), alive...)
				for i := len(order) - 1; i > 0; i-- {
					j := g.Intn(i + 1)
					order[i], order[j] = order[j], order[i]
				}
				if len(order)%2 == 1 {
					order = append(order, order[0]) // a ghost: the first club's line-up again
				}
				for p := 0; p < len(order); p += 2 {
					a, b := clubs[order[p]], clubs[order[p+1]]
					seed, _, _ := randomSeed()
					hex := fmt.Sprintf("%016x%016x", seed, uint64(k*131+round))
					la, _ := a.Squad("A")
					lb, _ := b.Squad("B")
					lus := [2]Lineup{la, lb}
					r, _ := SimulateSeed(hex, lus, [2]string{BotOrder(hex, lus, 0), BotOrder(hex, lus, 1)}, 2)
					a.SettleVersus(a.Round, "B", r.Score[0], r.Score[1], r.Winner == 0, len(r.Pens) > 0, r.SentOff, 0, round)
					if p+1 < len(order)-1 || len(alive)%2 == 0 {
						b.SettleVersus(b.Round, "A", r.Score[1], r.Score[0], r.Winner == 1, len(r.Pens) > 0, r.SentOff, 1, round)
					}
				}
				var still []int
				for _, i := range alive {
					if clubs[i].Patience > 0 {
						still = append(still, i)
					}
				}
				// Clubs out in the same round share the places they cover.
				for _, i := range alive {
					if clubs[i].Patience <= 0 {
						placed[ids[i]] = len(still) + 1
					}
				}
				alive = still
			}
			for _, i := range alive {
				placed[ids[i]] = 1
			}
			out <- res{placed}
		}(k)
	}
	for k := 0; k < lobbies; k++ {
		for id, p := range (<-out).place {
			sum[id] += float64(p)
			cnt[id]++
		}
	}
	type row struct {
		id  string
		avg float64
	}
	var rows []row
	for id := range sum {
		rows = append(rows, row{id, sum[id] / float64(cnt[id])})
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].avg < rows[b].avg })
	for _, r := range rows {
		t.Logf("%-22s average place %.2f (%d lobbies)", r.id, r.avg, cnt[r.id])
	}
}
