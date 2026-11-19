package game

import "testing"

// passesWhenThrough counts, per match, the passes played by an outfield player
// who had broken clear: well inside the opponent's half with no outfield
// opponent between him and the goal.
func passesWhenThrough(t *testing.T) float64 {
	t.Helper()
	const matches = 400
	n := 0
	for seed := uint64(1); seed <= matches; seed++ {
		res := Simulate(seed, [2]Lineup{weakTeam(), strongTeam()}, balanced, 2)
		for _, ev := range res.Events {
			if ev.Type != "pass" || ev.T < 1 || ev.T > 2*HalfTicks || ev.T > len(res.Frames) || res.Roster[ev.P].GK {
				continue
			}
			fr := res.Frames[ev.T-1]
			at := func(i int) vec { return vec{float64(fr[3+3*i]) / 10, float64(fr[3+3*i+1]) / 10} }
			me := at(ev.P)
			// A little stricter than the rule itself, so that a frame recorded a
			// moment after the decision cannot blur the picture.
			if adv(ev.Team, me.x) < throughFrom+3 {
				continue
			}
			goal := vec{goalX(ev.Team), PitchW / 2}
			clear := true
			for _, r := range res.Roster {
				if r.Team == ev.Team || r.GK || fr[3+3*r.Idx+2]&(1|8) != 0 {
					continue
				}
				if p := at(r.Idx); adv(ev.Team, p.x) > adv(ev.Team, me.x)-2 && segDist(me, goal, p) < throughLane+3 {
					clear = false
				}
			}
			if clear {
				n++
			}
		}
	}
	return float64(n) / matches
}

// A player who is clean through on goal keeps the ball and shoots; he does
// not turn round to look for a pass.
func TestNoPassWhenThrough(t *testing.T) {
	got := passesWhenThrough(t)
	t.Logf("passes played when clean through: %.3f per match", got)
	if got > 0.02 {
		t.Errorf("%.3f passes per match from a player who was clean through on goal, want almost none", got)
	}
}
