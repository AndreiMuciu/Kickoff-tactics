package game

import (
	"fmt"
	"testing"
)

// A top-four club plays on in the right cup; Europe costs no patience, group
// games may be drawn, knockout ties may not, and the run always ends.
func TestEuropeAfterTheLeague(t *testing.T) {
	cups := map[string]int{}
	for k := 0; k < 120; k++ {
		r, err := NewRun(fmt.Sprintf("cup-%d", k))
		if err != nil {
			t.Fatal(err)
		}
		g := NewRNG(uint64(k)*13 + 7)
		for r.Phase == "shop" {
			r.BotManage(g)
			if _, err := r.StartMatch(); err != nil {
				t.Fatal(err)
			}
			patience, m := r.Patience, r.cupMatch()
			_, h, err := r.FinishMatch("balanced")
			if err != nil {
				t.Fatal(err)
			}
			if h.Round <= MaxRounds {
				if h.Round == MaxRounds && r.Result != "sacked" {
					want := cupFor(r.League.Position())
					if got := ""; r.Cup != nil {
						got = r.Cup.ID
						if got != want {
							t.Fatalf("position %d plays in %q, want %q", r.League.Position(), got, want)
						}
					} else if want != "" {
						t.Fatalf("position %d did not go to Europe", r.League.Position())
					}
				}
				continue
			}
			cups[r.Cup.ID]++
			if r.Patience != patience || h.Damage != 0 {
				t.Fatalf("a European match cost patience: %d -> %d", patience, r.Patience)
			}
			if m > CupGroupDays && h.Draw {
				t.Fatalf("a knockout tie ended level: %+v", h)
			}
			if r.Round > MaxRounds+CupMatches {
				t.Fatalf("the run went past the final: round %d", r.Round)
			}
		}
		if r.Phase != "ended" {
			t.Fatalf("run %d stopped in phase %s", k, r.Phase)
		}
		if r.Cup != nil {
			switch r.Cup.Out {
			case "group", "semi", "final", "winner":
			default:
				t.Fatalf("European run ended without an outcome: %+v", r.Cup)
			}
			if r.View().Cup == nil || r.View().Cup.Group == nil {
				t.Fatal("the cup is missing from the view")
			}
		}
	}
	if len(cups) < 2 {
		t.Fatalf("too few cups played in 120 seasons: %v", cups)
	}
}
