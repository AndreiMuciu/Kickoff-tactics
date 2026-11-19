package game

import (
	"reflect"
	"testing"
)

func weakTeam() Lineup {
	return Lineup{Name: "weak", GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}, Units: []LineupUnit{
		{"maguire", 1, 0, 2}, {"kudus", 1, 1, 2}, {"nunez", 1, 2, 2}, {"pulisic", 1, 2, 0},
	}}
}

func strongTeam() Lineup {
	return Lineup{Name: "strong", GK: LineupUnit{DefID: "pickford", Stars: 1}, Units: []LineupUnit{
		{"bastoni", 1, 0, 2}, {"olmo", 1, 1, 2}, {"osimhen", 1, 2, 2}, {"leao", 1, 2, 4},
	}}
}

var balanced = [2][2]string{{"balanced", "balanced"}, {"balanced", "balanced"}}

func TestDeterministic(t *testing.T) {
	a := Simulate(42, [2]Lineup{weakTeam(), strongTeam()}, balanced, 2)
	b := Simulate(42, [2]Lineup{weakTeam(), strongTeam()}, balanced, 2)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different matches")
	}
}

// The first half must not depend on what is chosen at the break, otherwise a
// client could probe second-half tactics by replaying the first half.
func TestFirstHalfIsPrefix(t *testing.T) {
	lu := [2]Lineup{weakTeam(), strongTeam()}
	half := Simulate(7, lu, balanced, 1)
	full := Simulate(7, lu, [2][2]string{{"balanced", "all_out"}, {"balanced", "park_bus"}}, 2)
	if !reflect.DeepEqual(half.Frames, full.Frames[:HalfTicks]) {
		t.Fatal("first half frames differ")
	}
	if half.Score != full.HalfScore {
		t.Fatalf("half score %v != %v", half.Score, full.HalfScore)
	}
}

func TestBalance(t *testing.T) {
	const n = 2000
	strongWins, goals, shots, pens := 0, 0, 0, 0
	for i := 0; i < n; i++ {
		lu := [2]Lineup{weakTeam(), strongTeam()}
		strongSide := 1
		if i%2 == 1 {
			lu[0], lu[1] = lu[1], lu[0]
			strongSide = 0
		}
		r := Simulate(uint64(i)*7919+1, lu, balanced, 2)
		if r.Winner == strongSide {
			strongWins++
		}
		if len(r.Pens) > 0 {
			pens++
		}
		goals += r.Score[0] + r.Score[1]
		shots += r.Stats[0].Shots + r.Stats[1].Shots
	}
	rate := float64(strongWins) / n
	avg := float64(goals) / n
	t.Logf("strong win rate %.3f, goals/match %.2f, shots/match %.2f, shootouts %.2f", rate, avg, float64(shots)/n, float64(pens)/n)
	if rate < 0.6 || rate > 0.9 {
		t.Errorf("stronger team win rate %.2f outside 0.60-0.90", rate)
	}
	if avg < 1.5 || avg > 7 {
		t.Errorf("goals per match %.2f outside 1.5-7", avg)
	}
}

// Mirror match: neither side of the pitch may be favoured.
func TestSideFairness(t *testing.T) {
	const n = 3000
	w0 := 0
	for i := 0; i < n; i++ {
		r := Simulate(uint64(i)*104729+3, [2]Lineup{strongTeam(), strongTeam()}, balanced, 2)
		if r.Winner == 0 {
			w0++
		}
	}
	rate := float64(w0) / n
	t.Logf("team 0 win rate in mirror %.3f", rate)
	if rate < 0.45 || rate > 0.55 {
		t.Errorf("mirror win rate %.3f, pitch side is biased", rate)
	}
}

// Every trait must be reachable without breaking determinism or crashing.
func TestAllPlayersSimulate(t *testing.T) {
	var out []LineupUnit
	for _, p := range Players {
		if p.Role != "gk" {
			out = append(out, LineupUnit{p.ID, 1, len(out) % 3, len(out) % 5})
		}
	}
	for i, p := range Players {
		if p.Role != "gk" {
			continue
		}
		a := Lineup{GK: LineupUnit{DefID: p.ID, Stars: 2}, Units: out[(i*3)%20 : (i*3)%20+7]}
		b := Lineup{GK: LineupUnit{DefID: p.ID, Stars: 1}, Units: out[len(out)-7:]}
		r := Simulate(uint64(i)+9, [2]Lineup{a, b}, balanced, 2)
		if len(r.Frames) < 2*HalfTicks || r.Winner < 0 {
			t.Fatalf("bad result with keeper %s", p.ID)
		}
	}
}

// Penalties: awarded now and then for fouls in the box, mostly scored, and a
// drawn match gets a shootout that is recorded frame by frame.
func TestPenalties(t *testing.T) {
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	const n = 2000
	awarded, scored, shootouts, kicks := 0, 0, 0, 0
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
				lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1]})
			}
			return lu
		}
		r := Simulate(seed*41, [2]Lineup{mk(), mk()}, balanced, 2)
		pen := false
		for _, e := range r.Events {
			switch e.Type {
			case "penalty":
				awarded++
				pen = true
			case "goal", "save", "miss":
				if pen && e.T <= 2*HalfTicks {
					if e.Type == "goal" {
						scored++
					}
					pen = false
				}
			case "kickoff":
				pen = false
			}
		}
		if len(r.Pens) > 0 {
			shootouts++
			kicks += len(r.Pens)
			if len(r.Frames) != 2*HalfTicks+17*len(r.Pens) {
				t.Fatalf("seed %d: %d shootout frames for %d kicks", seed, len(r.Frames)-2*HalfTicks, len(r.Pens))
			}
			if last := r.Events[len(r.Events)-1]; last.T > len(r.Frames) {
				t.Fatalf("seed %d: shootout event at tick %d beyond %d frames", seed, last.T, len(r.Frames))
			}
		} else if len(r.Frames) != 2*HalfTicks {
			t.Fatalf("seed %d: %d frames without a shootout", seed, len(r.Frames))
		}
	}
	perMatch := float64(awarded) / n
	t.Logf("penalties per match %.2f, scored %.0f%%; shootouts in %.0f%% of matches, %.1f kicks each", perMatch, 100*float64(scored)/float64(max(1, awarded)), 100*float64(shootouts)/n, float64(kicks)/float64(max(1, shootouts)))
	if perMatch < 0.05 || perMatch > 0.6 {
		t.Errorf("penalties per match %.2f outside 0.05-0.60", perMatch)
	}
}

// Every real player belongs to a style, keepers included, and a keeper counts
// towards his style's combination.
func TestKeepersCountForStyles(t *testing.T) {
	known := map[string]bool{}
	for _, s := range Synergies {
		known[s.ID] = true
	}
	for _, p := range Players {
		if p.ID != AcademyKeeper && !known[p.Style] {
			t.Errorf("%s has style %q", p.ID, p.Style)
		}
	}
	lu := Lineup{GK: LineupUnit{DefID: "buffon", Stars: 1}, Units: []LineupUnit{{"maldini", 1, 0, 2}}}
	_, views := ComputeSynergies(lu)
	for _, v := range views {
		if v.ID == "catenaccio" && v.Count == 2 && v.Active == 0 {
			return
		}
	}
	t.Errorf("keeper not counted for his style: %+v", views)
}

// Fouls happen all over the pitch; yellows are common, reds rare, and a red
// always comes with a suspension of one to three matches (three seldom).
func TestCards(t *testing.T) {
	var ids []string
	for _, p := range Players {
		if p.Role != "gk" {
			ids = append(ids, p.ID)
		}
	}
	const n = 6000
	ev := map[string]int{}
	ban := map[int]int{}
	straight := 0
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
				lu.Units = append(lu.Units, LineupUnit{ids[g.Intn(len(ids))], 1, c[0], c[1]})
			}
			return lu
		}
		r := Simulate(seed*43, [2]Lineup{mk(), mk()}, balanced, 2)
		for _, e := range r.Events {
			ev[e.Type]++
		}
		for _, so := range r.SentOff {
			if so.Matches < 1 || so.Matches > 3 || (!so.Straight && so.Matches != 1) {
				t.Fatalf("seed %d: bad suspension %+v", seed, so)
			}
			if so.Straight {
				straight++
				ban[so.Matches]++
			}
		}
		// A dismissed player must be flagged off in the last frame.
		if len(r.SentOff) > 0 {
			off := 0
			last := r.Frames[2*HalfTicks-1]
			for i := range r.Roster {
				if last[5+i*3]&8 != 0 {
					off++
				}
			}
			if off != len(r.SentOff) {
				t.Fatalf("seed %d: %d sent off but %d flagged in frames", seed, len(r.SentOff), off)
			}
		}
	}
	f := func(k string) float64 { return float64(ev[k]) / n }
	t.Logf("per match: fouls %.2f, yellows %.2f, second yellows %.3f, straight reds %.3f, penalties %.2f", f("foul"), f("yellow"), f("red2"), f("red"), f("penalty"))
	t.Logf("straight-red bans: 1 match %.0f%%, 2 matches %.0f%%, 3 matches %.0f%%", 100*float64(ban[1])/float64(straight), 100*float64(ban[2])/float64(straight), 100*float64(ban[3])/float64(straight))
	if f("foul") < 0.5 || f("foul") > 4 || f("red")+f("red2") > 0.2 || f("red")+f("red2") < 0.01 {
		t.Errorf("card frequencies look wrong")
	}
	if !(ban[1] > ban[2] && ban[2] > ban[3] && ban[3] > 0) {
		t.Errorf("ban lengths should be mostly 1, sometimes 2, seldom 3: %v", ban)
	}
}
