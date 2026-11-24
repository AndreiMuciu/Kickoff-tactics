package game

import "testing"

// Thousands of random matches (any squad size, any stars, any order, players
// placed anywhere) must all tell a consistent story.
func TestMatchInvariants(t *testing.T) {
	all := outfieldPool(nil)
	var keepers []string
	for _, p := range Players {
		if p.Role == "gk" {
			keepers = append(keepers, p.ID)
		}
	}
	for seed := uint64(1); seed <= 4000; seed++ {
		g := NewRNG(seed * 131)
		mk := func() Lineup {
			lu := Lineup{GK: LineupUnit{DefID: keepers[g.Intn(len(keepers))], Stars: 1 + g.Intn(3)}}
			used, taken := map[[2]int]bool{}, map[string]bool{}
			for size := g.Intn(8); len(lu.Units) < size; {
				c, id := [2]int{g.Intn(3), g.Intn(5)}, all[g.Intn(len(all))]
				if used[c] || taken[id] {
					continue
				}
				used[c], taken[id] = true, true
				lu.Units = append(lu.Units, LineupUnit{id, 1 + g.Intn(3), c[0], c[1], ""})
			}
			return lu
		}
		order := func() string { return Tactics[g.Intn(len(Tactics))].ID }
		lus := [2]Lineup{mk(), mk()}
		r := Simulate(seed, lus, [2][2]string{{"balanced", order()}, {"balanced", order()}}, 2)
		fail := func(format string, a ...any) {
			t.Helper()
			t.Fatalf("seed %d (%d v %d players): "+format, append([]any{seed, len(lus[0].Units), len(lus[1].Units)}, a...)...)
		}
		// Score, goals and the winner agree.
		goals, pens := [2]int{}, [2]int{}
		lastT, off := 0, map[int]int{}
		kicks := 0
		for _, e := range r.Events {
			if e.T < lastT || e.T < 0 || e.T > len(r.Frames) {
				fail("event %q at tick %d is out of order or out of range (%d frames)", e.Type, e.T, len(r.Frames))
			}
			lastT = e.T
			switch e.Type {
			case "goal":
				goals[e.Team]++
			case "pengoal":
				pens[e.Team]++
			case "red", "red2":
				if _, twice := off[e.P]; twice {
					fail("player %d was sent off twice", e.P)
				}
				off[e.P] = e.T
			case "freekick":
				kicks++
				if r.Roster[e.P].GK || r.Roster[e.P].Team != e.Team {
					fail("free kick taken by the wrong player: %+v", r.Roster[e.P])
				}
			}
			if e.P >= len(r.Roster) || e.Q >= len(r.Roster) {
				fail("event %q names a player who is not in the roster", e.Type)
			}
			if _, gone := off[e.P]; gone && e.P >= 0 && e.T > off[e.P] && e.Type != "red" && e.Type != "red2" {
				fail("%s was sent off at tick %d and is in a %q event at tick %d", r.Roster[e.P].Name, off[e.P], e.Type, e.T)
			}
		}
		if goals != r.Score {
			fail("score %v but goal events %v", r.Score, goals)
		}
		if r.HalfScore[0] > r.Score[0] || r.HalfScore[1] > r.Score[1] {
			fail("half-time score %v is above the final score %v", r.HalfScore, r.Score)
		}
		switch {
		case r.Score[0] != r.Score[1]:
			if len(r.Pens) != 0 || (r.Score[0] > r.Score[1]) != (r.Winner == 0) {
				fail("score %v, winner %d, %d shootout kicks", r.Score, r.Winner, len(r.Pens))
			}
		case len(r.Pens) == 0 || pens[0] == pens[1] || (pens[0] > pens[1]) != (r.Winner == 0):
			fail("a draw %v must be settled on penalties: kicks %d, scored %v, winner %d", r.Score, len(r.Pens), pens, r.Winner)
		}
		if len(r.SentOff) != len(off) {
			fail("%d sendings-off recorded, %d red-card events", len(r.SentOff), len(off))
		}
		for team := 0; team < 2; team++ {
			if r.Stats[team].Shots < r.Score[team] {
				fail("team %d scored %d from %d shots", team, r.Score[team], r.Stats[team].Shots)
			}
		}
		// Frames: everyone on the pitch, the sent-off marked as gone.
		if len(r.Frames) < 2*HalfTicks {
			fail("only %d frames", len(r.Frames))
		}
		for i, fr := range r.Frames {
			if len(fr) != 3+3*len(r.Roster) {
				fail("frame %d has %d numbers for %d players", i, len(fr), len(r.Roster))
			}
			if fr[0] < -20 || fr[0] > int(PitchL*10)+20 || fr[1] < -20 || fr[1] > int(PitchW*10)+20 {
				fail("the ball left the stadium in frame %d: (%d, %d)", i, fr[0], fr[1])
			}
			for _, p := range r.Roster {
				x, y, flags := fr[3+3*p.Idx], fr[3+3*p.Idx+1], fr[3+3*p.Idx+2]
				gone := flags&8 != 0
				if at, sent := off[p.Idx]; sent && i >= at && !gone {
					fail("%s was sent off at tick %d and is still on the pitch in frame %d", p.Name, at, i)
				}
				if !gone && (x < 0 || x > int(PitchL*10) || y < 0 || y > int(PitchW*10)) {
					fail("%s is off the pitch in frame %d: (%d, %d)", p.Name, i, x, y)
				}
			}
		}
	}
}
