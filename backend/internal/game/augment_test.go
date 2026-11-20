package game

import (
	"reflect"
	"testing"
)

func TestAugmentOffers(t *testing.T) {
	for seed := uint64(1); seed <= 300; seed++ {
		r := &Run{Round: 1, Level: 3, RNGState: int64(seed)}
		for round := 1; round <= 10; round++ {
			r.Round = round
			r.withRNG(r.offerAugments)
			due := round == 2 || round == 5 || round == 8
			if !due {
				if len(r.AugmentOffer) != 0 {
					t.Fatalf("round %d: offer outside an augment round", round)
				}
				continue
			}
			if len(r.AugmentOffer) != augmentOffers {
				t.Fatalf("round %d: %d offers", round, len(r.AugmentOffer))
			}
			seen, crests := map[string]bool{}, 0
			for _, a := range r.AugmentOffer {
				if seen[a] || r.has(a) || AugmentByID[a] == nil {
					t.Fatalf("round %d: bad offer %v (owned %v)", round, r.AugmentOffer, r.Augments)
				}
				seen[a] = true
				if crestStyle(a) != "" {
					crests++
				}
			}
			if crests > 1 {
				t.Fatalf("more than one crest offered: %v", r.AugmentOffer)
			}
			if err := r.chooseAugment(int(seed) % augmentOffers); err != nil {
				t.Fatal(err)
			}
		}
		if len(r.Augments) != len(AugmentRounds) {
			t.Fatalf("seed %d: %d augments after a season", seed, len(r.Augments))
		}
	}
}

func TestAugmentEffects(t *testing.T) {
	r, _ := NewRun("t")
	r.Budget = 0
	r.AugmentOffer = []string{"sponsor", "windfall", "extraman"}
	if err := r.Apply(Action{Type: "augment", Slot: 1}); err != nil {
		t.Fatal(err)
	}
	if r.Budget != windfallBudget || len(r.AugmentOffer) != 0 {
		t.Fatalf("windfall: budget %d, offer %v", r.Budget, r.AugmentOffer)
	}
	mustRule(t, r.Apply(Action{Type: "augment", Slot: 0})) // nothing left to choose

	// Squad Rotation: one more outfield player than the level.
	r.Augments = append(r.Augments, "extraman")
	if r.Slots() != r.Level+1 {
		t.Fatalf("extra man: %d slots at level %d", r.Slots(), r.Level)
	}
	// Shrewd Investor raises the interest cap.
	r.Budget = 100
	if r.interest() != InterestMax {
		t.Fatalf("interest %d without the investor", r.interest())
	}
	r.Augments = append(r.Augments, "investor")
	if r.interest() != investorMax {
		t.Fatalf("interest %d with the investor", r.interest())
	}
	// Trusted by the Board: defeats cost less, never nothing.
	r.Augments = append(r.Augments, "trusted")
	if got := r.LossFor(20); got != 14 {
		t.Fatalf("trusted: 20 became %d", got)
	}
	if r.LossFor(1) != 1 {
		t.Fatal("a defeat must cost something")
	}
	// Scouting Network: the first refresh of each window is free.
	r.Augments = append(r.Augments, "scout")
	r.withRNG(r.nextWindow)
	r.Budget = 0
	if err := r.Apply(Action{Type: "reroll"}); err != nil {
		t.Fatalf("free refresh refused: %v", err)
	}
	mustRule(t, r.Apply(Action{Type: "reroll"})) // the second one costs again
}

func TestDeadlinePicksAnAugment(t *testing.T) {
	r, _ := NewRun("t")
	r.AugmentOffer = []string{"crowd", "fitness", "crest_tiki"}
	if !r.Deadline() || !r.has("crowd") || len(r.AugmentOffer) != 0 {
		t.Fatalf("deadline left %v, took %v", r.AugmentOffer, r.Augments)
	}
}

func TestCrestCountsAsAPlayer(t *testing.T) {
	lu := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}, Units: []LineupUnit{{DefID: "pedri", Stars: 1, Line: 1, Lane: 2}}}
	_, before := ComputeSynergies(lu)
	lu.Augments = []string{"crest_tiki"}
	_, after := ComputeSynergies(lu)
	count := func(v []SynergyView) (int, int) {
		for _, s := range v {
			if s.ID == "tiki" {
				return s.Count, s.Active
			}
		}
		return 0, -1
	}
	if c, a := count(before); c != 1 || a != -1 {
		t.Fatalf("before: %d, tier %d", c, a)
	}
	if c, a := count(after); c != 2 || a != 0 {
		t.Fatalf("after: %d, tier %d", c, a)
	}
	// A match augment changes the team's numbers.
	m0, _ := ComputeSynergies(Lineup{GK: lu.GK, Units: lu.Units})
	m1, _ := ComputeSynergies(Lineup{GK: lu.GK, Units: lu.Units, Augments: []string{"crowd", "backline"}})
	if m1.Stat[SPac] <= m0.Stat[SPac] || m1.OppXG >= m0.OppXG {
		t.Fatalf("match augments ignored: %+v vs %+v", m1, m0)
	}
}

func TestLockedMarketSurvivesOneWindow(t *testing.T) {
	r, _ := NewRun("t")
	shop := append([]string(nil), r.Shop...)
	if err := r.Apply(Action{Type: "lock"}); err != nil || !r.Locked {
		t.Fatal("lock refused")
	}
	r.withRNG(r.nextWindow)
	if !reflect.DeepEqual(r.Shop, shop) || r.Locked {
		t.Fatalf("locked market changed (%v -> %v) or stayed locked", shop, r.Shop)
	}
	// Unlocked, the next window brings a new market (five equal draws are near impossible).
	same := true
	for i := 0; i < 3 && same; i++ {
		r.withRNG(r.nextWindow)
		same = reflect.DeepEqual(r.Shop, shop)
	}
	if same {
		t.Fatal("market never changed after the lock was used")
	}
	// A refresh while locked unlocks.
	r.Budget = 10
	_ = r.Apply(Action{Type: "lock"})
	_ = r.Apply(Action{Type: "reroll"})
	if r.Locked {
		t.Fatal("refresh kept the lock")
	}
}

func TestDefeatsCostGradually(t *testing.T) {
	prev := 0
	for stake := 1; stake <= 16; stake++ {
		d := VersusDamage(stake, 1)
		if d <= prev {
			t.Fatalf("stake %d costs %d, not more than %d", stake, d, prev)
		}
		prev = d
		if VersusDamage(stake, 7) != VersusDamage(stake, 3) {
			t.Fatal("margins beyond three goals must not cost more")
		}
	}
	// Two early defeats must not end a lobby; the first rounds stay cheap.
	if VersusDamage(1, 3)+VersusDamage(2, 3) >= VersusPatience/2 {
		t.Fatalf("early defeats cost %d", VersusDamage(1, 3)+VersusDamage(2, 3))
	}
}

func TestShootoutRound(t *testing.T) {
	a := Lineup{GK: LineupUnit{DefID: "buffon", Stars: 1}, Units: []LineupUnit{{DefID: "kane", Stars: 1, Line: 2, Lane: 2}}}
	b := Lineup{GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}} // keeper alone: he takes the kicks
	wins := 0
	for i := 0; i < 200; i++ {
		seed, _, _ := NewSeed()
		x, err := SimulateShootout(seed, [2]Lineup{a, b})
		if err != nil {
			t.Fatal(err)
		}
		y, _ := SimulateShootout(seed, [2]Lineup{a, b})
		if !reflect.DeepEqual(x.Pens, y.Pens) || len(x.Frames) != len(y.Frames) {
			t.Fatal("shootout not deterministic")
		}
		if len(x.Pens) < 2*cupKicks || x.Score != [2]int{} || x.Winner < 0 || len(x.Frames) == 0 {
			t.Fatalf("bad shootout: %d kicks, score %v, winner %d", len(x.Pens), x.Score, x.Winner)
		}
		for _, ev := range x.Events {
			if ev.T < 2*HalfTicks || ev.T > 2*HalfTicks+len(x.Frames) {
				t.Fatalf("event at tick %d outside the shootout frames", ev.T)
			}
		}
		if x.Winner == 0 {
			wins++
		}
	}
	if wins < 100 {
		t.Fatalf("Kane against a keeper taking his own kicks won only %d of 200", wins)
	}
}

func TestBanFollowsAMovedPlayer(t *testing.T) {
	r, _ := NewRun("t")
	r.Units = append(r.Units, Unit{UID: "u9", Def: "kane", Stars: 1, Line: -1, Lane: 3})
	r.Discipline([]SentOff{{Team: 0, Def: "kane", Line: 2, Lane: 2, Matches: 2}}, 0)
	if r.find("u9").Ban != 2 {
		t.Fatal("a player moved during the match escaped his ban")
	}
}
