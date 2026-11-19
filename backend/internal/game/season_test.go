package game

import (
	"errors"
	"testing"
)

// Plays whole seasons with random management, the way a player would, and
// fails on any non-rule error or panic.
func TestFullSeasons(t *testing.T) {
	tactics := []string{"balanced", "all_out", "park_bus", "press", "keep_ball"}
	ended := map[string]int{}
	for seed := uint64(1); seed <= 400; seed++ {
		g := NewRNG(seed)
		r, err := NewRun("s")
		if err != nil {
			t.Fatal(err)
		}
		for step := 0; r.Phase != "ended"; step++ {
			if step > 60 {
				t.Fatalf("seed %d: season did not end (round %d phase %s)", seed, r.Round, r.Phase)
			}
			for k := 0; k < 25; k++ {
				var a Action
				switch g.Intn(6) {
				case 0, 1:
					a = Action{Type: "buy", Slot: g.Intn(ShopSize)}
				case 2:
					a = Action{Type: "levelup"}
				case 3:
					a = Action{Type: "reroll"}
				default:
					if len(r.Units) > 0 {
						u := r.Units[g.Intn(len(r.Units))]
						if g.Intn(8) == 0 {
							a = Action{Type: "sell", UID: u.UID}
						} else {
							line := g.Intn(5) - 1
							a = Action{Type: "move", UID: u.UID, Line: line, Lane: g.Intn(5)}
						}
					}
				}
				if err := r.Apply(a); err != nil {
					var re ErrRule
					if !errors.As(err, &re) {
						t.Fatalf("seed %d: %v", seed, err)
					}
				}
			}
			// Like a real manager at the deadline: if nobody eligible is on the
			// pitch, bring someone on. With nobody eligible at all, the match is
			// played by the keeper alone and must still start.
			r.AutoField()
			if board, slots := func() (int, int) { _, b := r.counts(); return b, r.Slots() }(); board > slots {
				t.Fatalf("seed %d: %d outfield players with %d slots", seed, board, slots)
			}
			if _, err := r.StartMatch(); err != nil {
				t.Fatalf("seed %d round %d: start: %v", seed, r.Round, err)
			}
			if _, _, err := r.FinishMatch(tactics[g.Intn(len(tactics))]); err != nil {
				t.Fatalf("seed %d round %d: finish: %v", seed, r.Round, err)
			}
			gks := 0
			for _, u := range r.Units {
				if u.Line == GKLine {
					gks++
				}
			}
			if gks != 1 {
				t.Fatalf("seed %d: %d keepers in goal", seed, gks)
			}
		}
		ended[r.Result]++
	}
	t.Logf("results over 400 random seasons: %v", ended)
}
