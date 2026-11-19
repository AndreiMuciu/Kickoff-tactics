package game

import "testing"

// The same squad on both sides must win about half the time from either half
// of the pitch, also when its formation is lopsided.
func TestMirrorLarge(t *testing.T) {
	for _, name := range []string{"strong", "weak"} {
		lu := strongTeam()
		if name == "weak" {
			lu = weakTeam()
		}
		const n = 12000
		w0, pens, g0, g1 := 0, 0, 0, 0
		for i := 0; i < n; i++ {
			r := Simulate(uint64(i)*2654435761+11, [2]Lineup{lu, lu}, balanced, 2)
			if r.Winner == 0 {
				w0++
			}
			if len(r.Pens) > 0 {
				pens++
			}
			g0 += r.Score[0]
			g1 += r.Score[1]
		}
		if rate := float64(w0) / n; rate < 0.48 || rate > 0.52 {
			t.Errorf("%s mirror: team 0 wins %.3f, pitch side is biased", name, rate)
		}
		t.Logf("%s mirror: team 0 wins %.3f, goals %.3f vs %.3f, shootouts %.3f", name, float64(w0)/n, float64(g0)/n, float64(g1)/n, float64(pens)/n)
	}
}
