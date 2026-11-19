package game

import "testing"

// advancedShare is how often the player `def` of team 0 stands beyond `x`
// (towards the opponent's goal) during normal play.
func advancedShare(t *testing.T, my Lineup, tactics [2][2]string, def string, x float64) float64 {
	t.Helper()
	beyond, total := 0, 0
	for seed := uint64(1); seed <= 150; seed++ {
		res := Simulate(seed, [2]Lineup{my, strongTeam()}, tactics, 2)
		idx := -1
		for _, r := range res.Roster {
			if r.Team == 0 && r.Def == def {
				idx = r.Idx
			}
		}
		if idx < 0 {
			t.Fatalf("%s is not in the roster", def)
		}
		for i, fr := range res.Frames {
			if i >= 2*HalfTicks || fr[3+3*idx+2]&8 != 0 { // not the shootout, not after a red card
				continue
			}
			total++
			if adv(0, float64(fr[3+3*idx])/10) > x {
				beyond++
			}
		}
	}
	return float64(beyond) / float64(total)
}

// Where a player is placed decides how far he goes: on the defence line, with
// team-mates ahead of him, he stays in and around his own half.
func TestBackLineHolds(t *testing.T) {
	if got := advancedShare(t, weakTeam(), balanced, "maguire", 62); got > 0.005 {
		t.Errorf("a defender with team-mates ahead is beyond 62 m in %.1f%% of frames, want almost never", 100*got)
	}
	// The same player placed in attack spends a lot of the match up there.
	up := weakTeam()
	up.Units[0].Line = 2
	up.Units[0].Lane = 3
	if got := advancedShare(t, up, balanced, "maguire", 62); got < 0.15 {
		t.Errorf("the same player placed in attack is beyond 62 m in only %.1f%% of frames", 100*got)
	}
	// Alone on the pitch he cannot hold a line: he has to go forward himself.
	alone := Lineup{Name: "alone", GK: weakTeam().GK, Units: weakTeam().Units[:1]}
	if got := advancedShare(t, alone, balanced, "maguire", 62); got < 0.02 {
		t.Errorf("a lone defender is beyond 62 m in only %.1f%% of frames: he should play the whole pitch", 100*got)
	}
	// All-Out Attack sends the back line forward with everyone else.
	allOut := [2][2]string{{"all_out", "all_out"}, {"balanced", "balanced"}}
	if got := advancedShare(t, weakTeam(), allOut, "maguire", 56); got < 0.03 {
		t.Errorf("under All-Out Attack the defender is beyond 56 m in only %.1f%% of frames", 100*got)
	}
}
