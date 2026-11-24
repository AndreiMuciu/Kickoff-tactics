package game

import (
	"fmt"
	"testing"
)

// After the league: a top-four club goes to the right cup, relegated clubs to
// the play-off. Europe costs no patience, knockout ties are never level, the
// whole cup is told to the end, and the play-off sacks on any defeat.
func TestRunAfterTheLeague(t *testing.T) {
	seen := map[string]int{}
	for k := 0; k < 200; k++ {
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
			inEurope := r.Cup != nil && !CupByID[r.Cup.ID].Playoff
			_, h, err := r.FinishMatch("balanced")
			if err != nil {
				t.Fatal(err)
			}
			if h.Round == MaxRounds && r.Result != "sacked" {
				want, got := cupFor(r.League.Position()), ""
				if r.Cup != nil {
					got = r.Cup.ID
				}
				if got != want {
					t.Fatalf("position %d goes to %q, want %q", r.League.Position(), got, want)
				}
				if got == PlayoffID && r.Patience != 1 {
					t.Fatalf("play-off patience %d", r.Patience)
				}
			}
			if h.Round <= MaxRounds {
				continue
			}
			if inEurope && (r.Patience != patience || h.Damage != 0) {
				t.Fatalf("a European match cost patience: %d -> %d", patience, r.Patience)
			}
			if inEurope && m > CupGroupDays && h.Draw {
				t.Fatalf("a knockout tie ended level: %+v", h)
			}
			if !inEurope && !h.Won && !h.Draw && r.Cup.Out != "sacked" {
				t.Fatalf("a play-off defeat did not end it: %+v", r.Cup)
			}
		}
		if r.Phase != "ended" {
			t.Fatalf("run %d stopped in phase %s", k, r.Phase)
		}
		c := r.Cup
		if c == nil {
			continue
		}
		seen[c.ID+":"+c.Out]++
		v := r.View().Cup
		if CupByID[c.ID].Playoff {
			if c.Out == "promoted" && c.Groups[0].Position() != 1 {
				t.Fatalf("promoted from position %d", c.Groups[0].Position())
			}
			continue
		}
		names := map[string]bool{}
		for _, lg := range c.Groups {
			if len(lg.Teams) != 4 {
				t.Fatalf("a group of %d", len(lg.Teams))
			}
			for _, tm := range lg.Teams {
				if names[tm.Name] {
					t.Fatalf("%s drawn twice", tm.Name)
				}
				names[tm.Name] = true
				if tm.P != CupGroupDays {
					t.Fatalf("%s played %d group games", tm.Name, tm.P)
				}
			}
		}
		if len(v.Groups) != 4 || len(c.Ties) != 7 || v.Winner == "" {
			t.Fatalf("the cup was not told to the end: %d groups, %d ties, winner %q, out %q", len(v.Groups), len(c.Ties), v.Winner, c.Out)
		}
		if (c.Out == "winner") != (v.Winner == YourClub) {
			t.Fatalf("out %q but the winner is %s", c.Out, v.Winner)
		}
	}
	t.Logf("outcomes: %v", seen)
	if len(seen) < 4 {
		t.Fatalf("too few different outcomes: %v", seen)
	}
}

// The draws are random: the same cup gives different groups.
func TestCupDrawIsRandom(t *testing.T) {
	a, b := newCup(NewRNG(1), "plate"), newCup(NewRNG(2), "plate")
	if a.Groups[a.Mine].Teams[1].Name == b.Groups[b.Mine].Teams[1].Name && a.Groups[0].Teams[1].Name == b.Groups[0].Teams[1].Name {
		t.Fatal("two draws gave the same groups")
	}
}

// The daily challenge: the same start for everyone on a day, a different one
// the next day, and the season ends with the league.
func TestDailyChallenge(t *testing.T) {
	a, b, c := NewDailyRun("a", "2026-11-24"), NewDailyRun("b", "2026-11-24"), NewDailyRun("c", "2026-11-25")
	if fmt.Sprint(a.Shop, a.League.Teams, a.Opponent) != fmt.Sprint(b.Shop, b.League.Teams, b.Opponent) {
		t.Fatal("two managers got different daily challenges on the same day")
	}
	if fmt.Sprint(a.Shop, a.League.Teams) == fmt.Sprint(c.Shop, c.League.Teams) {
		t.Fatal("the next day repeats the challenge")
	}
	g := NewRNG(5)
	for a.Phase == "shop" {
		a.BotManage(g)
		if _, err := a.StartMatch(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := a.FinishMatch("balanced"); err != nil {
			t.Fatal(err)
		}
	}
	if a.Cup != nil || a.Round > MaxRounds || a.View().Daily != "2026-11-24" {
		t.Fatalf("the daily went on after the league: round %d, cup %v", a.Round, a.Cup)
	}
}
