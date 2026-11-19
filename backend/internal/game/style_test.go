package game

import "testing"

func lineupWith(gk string, ids ...string) Lineup {
	lu := Lineup{Name: "test", GK: LineupUnit{DefID: gk, Stars: 1}}
	for i, id := range ids {
		lu.Units = append(lu.Units, LineupUnit{DefID: id, Stars: 1, Line: 1 + i%2, Lane: i % 5})
	}
	return lu
}

// Every style is staffed the same way, so no style is easier to complete.
func TestPoolShape(t *testing.T) {
	outfield, keepers, traits := map[string]int{}, map[string]int{}, map[string]string{}
	for _, p := range Players {
		if other, dup := traits[p.Trait]; dup {
			t.Errorf("%s and %s share the trait %q: traits are unique", p.ID, other, p.Trait)
		}
		traits[p.Trait] = p.ID
		switch {
		case p.ID == AcademyKeeper:
		case p.Role == "gk":
			keepers[p.Style]++
		default:
			outfield[p.Style]++
		}
	}
	styles := 0
	for _, s := range Synergies {
		if s.Kind != "style" {
			continue
		}
		styles++
		if outfield[s.ID] != 12 || keepers[s.ID] != 2 {
			t.Errorf("%s has %d outfield players and %d keepers, want 12 and 2", s.Name, outfield[s.ID], keepers[s.ID])
		}
		if len(s.Tiers) != 3 || len(s.Desc) != 3 {
			t.Errorf("%s should have three steps (2, 4, 6)", s.Name)
		}
	}
	if styles != 8 {
		t.Errorf("%d styles, want 8", styles)
	}
}

func TestStyleSteps(t *testing.T) {
	step := func(lu Lineup, id string) int {
		_, views := ComputeSynergies(lu)
		for _, v := range views {
			if v.ID == id {
				return v.Active
			}
		}
		return -2
	}
	six := lineupWith("onana", "ferran", "cucurella", "zubimendi", "olmo", "pedri")
	if got := step(six, "tiki"); got != 2 {
		t.Errorf("five Tiki-Taka players and their keeper should reach the third step, got step index %d", got)
	}
	m, _ := ComputeSynergies(lineupWith(AcademyKeeper, "bernardo", "alaba", "dejong", "kimmich"))
	for k, v := range m.Stat {
		if v < 1.039 {
			t.Errorf("Total Football at 4 should lift every stat by 4%%; stat %d is x%.3f", k, v)
		}
	}
	if m, _ := ComputeSynergies(lineupWith(AcademyKeeper, "pulisic", "werner", "vardy", "hakimi")); m.Counter != 1.1 {
		t.Errorf("Counter at 4 should give a 1.1 s sprint, got %.1f", m.Counter)
	}
	m, _ = ComputeSynergies(lineupWith(AcademyKeeper, "wardprowse", "eriksen", "bruno", "beckham"))
	if m.FoulDraw != 1.6 || m.PenBonus != 0.10 || m.FreeKick <= 0 {
		t.Errorf("Set Pieces at 4: fouls x%.1f, penalties +%.2f, free kick %.2f", m.FoulDraw, m.PenBonus, m.FreeKick)
	}
}

// setPieceNumbers plays many matches and reports, per match for team 0: fouls
// suffered, direct free kicks taken and goals scored from them, penalties.
func setPieceNumbers(my Lineup) (fouls, kicks, goals, pens float64) {
	const n = 1500
	for seed := uint64(1); seed <= n; seed++ {
		res := Simulate(seed, [2]Lineup{my, strongTeam()}, balanced, 2)
		pending := false // a free kick has been struck and its outcome is still to come
		for _, ev := range res.Events {
			switch {
			case ev.Type == "foul" && ev.Team == 1:
				fouls++
			case ev.Type == "penalty" && ev.Team == 0:
				pens++
			case ev.Type == "freekick" && ev.Team == 0:
				kicks++
				pending = true
			case pending && ev.Type == "goal" && ev.Team == 0:
				goals++
				pending = false
			case pending && (ev.Type == "save" || ev.Type == "miss"):
				pending = false
			}
		}
	}
	return fouls / n, kicks / n, goals / n, pens / n
}

// A Set Pieces team is fouled more, wins more penalties and shoots from free
// kicks; a team without the style never shoots from one.
func TestSetPieces(t *testing.T) {
	plain := lineupWith(AcademyKeeper, "kudus", "nunez", "pulisic", "ferran")
	set := lineupWith(AcademyKeeper, "wardprowse", "eriksen", "bruno", "beckham")
	f0, k0, _, p0 := setPieceNumbers(plain)
	f1, k1, g1, p1 := setPieceNumbers(set)
	t.Logf("no style:   fouled %.2f, free kicks at goal %.2f, penalties %.3f per match", f0, k0, p0)
	t.Logf("Set Pieces: fouled %.2f, free kicks at goal %.2f (%.3f goals), penalties %.3f per match", f1, k1, g1, p1)
	if k0 != 0 {
		t.Errorf("a team with neither the style nor a specialist took %.2f direct free kicks per match", k0)
	}
	// One specialist is enough to go for goal, even without the style.
	_, k2, g2, _ := setPieceNumbers(lineupWith(AcademyKeeper, "kudus", "nunez", "pulisic", "beckham"))
	t.Logf("Beckham alone: free kicks at goal %.2f (%.3f goals) per match", k2, g2)
	if k2 <= 0 {
		t.Errorf("a free-kick specialist without the style never struck a free kick at goal")
	}
	if f1 < f0*1.25 {
		t.Errorf("Set Pieces at 4 should be fouled clearly more often: %.2f against %.2f", f1, f0)
	}
	if k1 < 0.05 {
		t.Errorf("Set Pieces at 4 took only %.3f direct free kicks per match", k1)
	}
	if k1 > 0 && (g1/k1 < 0.05 || g1/k1 > 0.45) {
		t.Errorf("%.0f%% of direct free kicks score: want a real chance, not a penalty", 100*g1/k1)
	}
}
