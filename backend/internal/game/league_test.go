package game

import (
	"fmt"
	"testing"
)

// Every matchday each club plays once; over the season everyone meets
// everyone exactly once, and you meet the rivals in the order of clubNames.
func TestLeagueFixtures(t *testing.T) {
	met := map[[2]int]bool{}
	for round := 1; round <= MaxRounds; round++ {
		seen := map[int]bool{}
		pairs := fixtures(round)
		if len(pairs) != LeagueTeams/2 {
			t.Fatalf("matchday %d has %d matches", round, len(pairs))
		}
		if pairs[0] != [2]int{0, round} {
			t.Fatalf("matchday %d: you meet %v, want rival %d", round, pairs[0], round)
		}
		for _, p := range pairs {
			if seen[p[0]] || seen[p[1]] || p[0] == p[1] {
				t.Fatalf("matchday %d: a club plays twice: %v", round, pairs)
			}
			seen[p[0]], seen[p[1]] = true, true
			k := [2]int{min(p[0], p[1]), max(p[0], p[1])}
			if met[k] {
				t.Fatalf("%v meet twice", k)
			}
			met[k] = true
		}
	}
	if len(met) != LeagueTeams*(LeagueTeams-1)/2 {
		t.Fatalf("%d pairings, want a full round robin", len(met))
	}
}

// A whole season keeps the table consistent and the result follows the position.
func TestLeagueSeason(t *testing.T) {
	for k := 0; k < 40; k++ {
		r, _ := NewRun(fmt.Sprintf("league-%d", k))
		g := NewRNG(uint64(k) + 3)
		for r.Phase == "shop" {
			r.BotManage(g)
			if _, err := r.StartMatch(); err != nil {
				t.Fatal(err)
			}
			if _, _, err := r.FinishMatch("balanced"); err != nil {
				t.Fatal(err)
			}
		}
		played := r.Wins + r.Draws + r.Losses
		goals := 0
		for _, tm := range r.League.Teams {
			if tm.P != played || tm.W+tm.D+tm.L != tm.P || tm.Pts != 3*tm.W+tm.D {
				t.Fatalf("season %d: bad row %+v after %d matchdays", k, tm, played)
			}
			goals += tm.GF - tm.GA
			if tm.You && (tm.W != r.Wins || tm.D != r.Draws || tm.L != r.Losses) {
				t.Fatalf("season %d: your row %+v vs record %d-%d-%d", k, tm, r.Wins, r.Draws, r.Losses)
			}
		}
		if goals != 0 {
			t.Fatalf("season %d: goal difference sums to %d", k, goals)
		}
		if r.Result != "sacked" {
			if want := r.finalResult(); r.Result != want || played != MaxRounds {
				t.Fatalf("season %d: result %q at position %d", k, r.Result, r.League.Position())
			}
		}
	}
}
