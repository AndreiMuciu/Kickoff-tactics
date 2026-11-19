package game

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func mustRule(t *testing.T, err error) {
	t.Helper()
	var e ErrRule
	if !errors.As(err, &e) {
		t.Fatalf("expected a rule error, got %v", err)
	}
}

func TestActionsAreValidated(t *testing.T) {
	r, err := NewRun("s")
	if err != nil {
		t.Fatal(err)
	}
	mustRule(t, r.Apply(Action{Type: "buy", Slot: 99}))
	mustRule(t, r.Apply(Action{Type: "buy", Slot: -1}))
	mustRule(t, r.Apply(Action{Type: "sell", UID: "nope"}))
	mustRule(t, r.Apply(Action{Type: "hack"}))
	mustRule(t, r.Apply(Action{Type: "move", UID: "nope", Line: 0, Lane: 0}))

	r.Budget = 0
	mustRule(t, r.Apply(Action{Type: "buy", Slot: 0}))
	mustRule(t, r.Apply(Action{Type: "reroll"}))
	mustRule(t, r.Apply(Action{Type: "levelup"}))
	if r.Budget != 0 {
		t.Fatalf("budget changed to %d", r.Budget)
	}

	r.Budget = 100
	r.Shop = []string{"nunez", "nunez", "kudus", "maguire", "man"}
	for i := 0; i < 5; i++ {
		if err := r.Apply(Action{Type: "buy", Slot: i}); err != nil {
			t.Fatal(err)
		}
	}
	mustRule(t, r.Apply(Action{Type: "buy", Slot: 0})) // already bought
	uid := r.Units[1].UID
	mustRule(t, r.Apply(Action{Type: "move", UID: uid, Line: 3, Lane: 0}))
	mustRule(t, r.Apply(Action{Type: "move", UID: uid, Line: 0, Lane: 5}))
	// One outfield player per stadium level.
	placed := 0
	for i, u := range append([]Unit{}, r.Units[1:]...) {
		err := r.Apply(Action{Type: "move", UID: u.UID, Line: 0, Lane: i})
		if err == nil {
			placed++
		} else {
			mustRule(t, err)
		}
	}
	if placed != r.Slots() {
		t.Fatalf("placed %d, stadium allows %d", placed, r.Slots())
	}
}

// A purchase names the player the manager saw. If the market on his screen is
// out of date, nothing is bought and nothing is paid.
func TestBuyChecksWhoIsOnSale(t *testing.T) {
	r, _ := NewRun("s")
	r.Budget = 100
	r.Shop = []string{"nunez", "kudus", "", "", ""}
	mustRule(t, r.Apply(Action{Type: "buy", Slot: 0, Def: "kudus"}))
	if r.Budget != 100 || r.Shop[0] != "nunez" {
		t.Fatalf("a refused purchase changed the club: budget %d, market %v", r.Budget, r.Shop)
	}
	if err := r.Apply(Action{Type: "buy", Slot: 0, Def: "nunez"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(Action{Type: "buy", Slot: 1}); err != nil { // bots do not name the player
		t.Fatal(err)
	}
}

// A suspended player on the bench is never brought on automatically, however
// much room the pitch has; once his ban is served he is eligible again.
func TestAutoFieldSkipsSuspended(t *testing.T) {
	r, _ := NewRun("s")
	r.Level, r.Budget = 3, 100
	r.Shop = []string{"nunez", "kudus", "", "", ""}
	for i := 0; i < 2; i++ {
		if err := r.Apply(Action{Type: "buy", Slot: i}); err != nil {
			t.Fatal(err)
		}
	}
	r.Units[1].Ban = 1 // richarlison, first on the bench
	r.AutoField()
	if r.Units[1].Line >= 0 {
		t.Fatalf("a suspended player was brought on: %+v", r.Units[1])
	}
	if r.Units[2].Line != 1 { // Kudus is a playmaker: he belongs in midfield
		t.Fatalf("the eligible bench player was not brought on in midfield: %+v", r.Units[2])
	}
	if _, err := r.StartMatch(); err != nil {
		t.Fatal(err)
	}
	for _, u := range r.Match.My.Units {
		if u.DefID == "nunez" {
			t.Fatal("a suspended player is in the line-up")
		}
	}
	r.Discipline(nil, 0) // the match he sat out
	if r.Units[1].Ban != 0 || !r.AutoField() || r.Units[1].Line < 0 {
		t.Fatalf("with the ban served he should come on again: %+v", r.Units[1])
	}
}

func TestMerge(t *testing.T) {
	r, _ := NewRun("s")
	r.Budget = 100
	for i := 0; i < 3; i++ {
		r.Shop = []string{"nunez", "", "", "", ""}
		if err := r.Apply(Action{Type: "buy", Slot: 0}); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.Units) != 2 || r.Units[1].Stars != 2 {
		t.Fatalf("expected one 2-star unit, got %+v", r.Units)
	}
}

func TestMatchFlowAndCommitment(t *testing.T) {
	r, _ := NewRun("s")
	r.Level = 3 // three outfield slots
	r.Shop = []string{"nunez", "kudus", "maguire", "", ""}
	for i := 0; i < 3; i++ {
		if err := r.Apply(Action{Type: "buy", Slot: i}); err != nil {
			t.Fatal(err)
		}
	}
	// Free places are filled from the bench, in bench order, up to the stadium level.
	if !r.AutoField() {
		t.Fatal("bench players were not brought on to an empty pitch")
	}
	if _, board := r.counts(); board != min(3, r.Slots()) {
		t.Fatalf("%d on the pitch with %d slots and three on the bench", board, r.Slots())
	}
	if r.AutoField() {
		t.Fatal("auto-field changed a pitch that was already full")
	}
	for i, u := range append([]Unit{}, r.Units[1:]...) {
		if err := r.Apply(Action{Type: "move", UID: u.UID, Line: i, Lane: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := r.FinishMatch("balanced"); err == nil {
		t.Fatal("second half played before kickoff")
	}
	first, err := r.StartMatch()
	if err != nil {
		t.Fatal(err)
	}
	if v := r.View(); v.Commit == "" || v.Phase != "halftime" {
		t.Fatalf("bad halftime view: %+v", v)
	}
	mustRule(t, r.Apply(Action{Type: "reroll"})) // no management during a match
	if _, err := r.StartMatch(); err == nil {
		t.Fatal("match restarted mid-game")
	}
	if _, _, err := r.FinishMatch("cheat"); err == nil {
		t.Fatal("unknown tactic accepted")
	}
	res, h, err := r.FinishMatch("all_out")
	if err != nil {
		t.Fatal(err)
	}
	if res.HalfScore != first.Score {
		t.Fatalf("first half changed: %v vs %v", res.HalfScore, first.Score)
	}
	sum := sha256.Sum256([]byte(h.Seed))
	if hex.EncodeToString(sum[:]) != h.Commit {
		t.Fatal("revealed seed does not match the commitment")
	}
	if r.Phase != "shop" || r.Round != 2 || r.Match != nil {
		t.Fatalf("bad state after match: phase=%s round=%d", r.Phase, r.Round)
	}
}

func TestGoalkeeperRules(t *testing.T) {
	r, _ := NewRun("s")
	gk := r.Units[0]
	if gk.Def != AcademyKeeper || gk.Line != GKLine {
		t.Fatalf("no starting keeper: %+v", r.Units)
	}
	mustRule(t, r.Apply(Action{Type: "sell", UID: gk.UID}))                    // never without a keeper
	mustRule(t, r.Apply(Action{Type: "move", UID: gk.UID, Line: -1, Lane: 0})) // cannot leave the goal empty
	mustRule(t, r.Apply(Action{Type: "move", UID: gk.UID, Line: 1, Lane: 2}))  // keepers do not play outfield

	r.Budget = 100
	r.Shop = []string{"pickford", "nunez", "", "", ""}
	for i := 0; i < 2; i++ {
		if err := r.Apply(Action{Type: "buy", Slot: i}); err != nil {
			t.Fatal(err)
		}
	}
	nita, striker := r.Units[1], r.Units[2]
	mustRule(t, r.Apply(Action{Type: "move", UID: striker.UID, Line: GKLine, Lane: 0}))   // outfielder in goal
	mustRule(t, r.Apply(Action{Type: "move", UID: nita.UID, Line: 0, Lane: 0}))           // keeper outfield
	mustRule(t, r.Apply(Action{Type: "move", UID: gk.UID, Line: -1, Lane: striker.Lane})) // swap with a non-keeper

	// Swap: the new keeper goes in goal, the old one lands on the bench.
	if err := r.Apply(Action{Type: "move", UID: nita.UID, Line: GKLine, Lane: 0}); err != nil {
		t.Fatal(err)
	}
	if u := r.find(nita.UID); u.Line != GKLine {
		t.Fatalf("new keeper not in goal: %+v", u)
	}
	if u := r.find(gk.UID); u.Line != -1 {
		t.Fatalf("old keeper not on bench: %+v", u)
	}
	// Now the benched keeper can be sold, the one in goal still cannot.
	if err := r.Apply(Action{Type: "sell", UID: gk.UID}); err != nil {
		t.Fatal(err)
	}
	mustRule(t, r.Apply(Action{Type: "sell", UID: nita.UID}))
	if lu := r.myLineup(); lu.GK.DefID != "pickford" {
		t.Fatalf("lineup keeper is %s", lu.GK.DefID)
	}
}

func TestStadiumLevelIsOutfieldCount(t *testing.T) {
	r, _ := NewRun("s")
	if r.Level != 1 || r.Slots() != 1 {
		t.Fatalf("level %d gives %d outfield slots", r.Level, r.Slots())
	}
	r.Budget = 1000
	for r.Level < MaxLevel {
		lvl := r.Level
		for r.Level == lvl {
			if err := r.Apply(Action{Type: "levelup"}); err != nil {
				t.Fatal(err)
			}
		}
		if r.Slots() != r.Level {
			t.Fatalf("level %d gives %d slots", r.Level, r.Slots())
		}
	}
	mustRule(t, r.Apply(Action{Type: "levelup"}))
}

func TestSuspensions(t *testing.T) {
	r, _ := NewRun("s")
	r.Budget, r.Level = 100, 2
	r.Shop = []string{"nunez", "kudus", "maguire", "", ""}
	for i := 0; i < 3; i++ {
		if err := r.Apply(Action{Type: "buy", Slot: i}); err != nil {
			t.Fatal(err)
		}
	}
	a, b, c := r.Units[1].UID, r.Units[2].UID, r.Units[3].UID
	_ = r.Apply(Action{Type: "move", UID: a, Line: 2, Lane: 2})
	_ = r.Apply(Action{Type: "move", UID: b, Line: 1, Lane: 2})
	// The striker is sent off for two matches.
	r.Discipline([]SentOff{{Team: 0, Def: "nunez", Line: 2, Lane: 2, Straight: true, Matches: 2}, {Team: 1, Def: "kudus", Line: 1, Lane: 2, Matches: 1}}, 0)
	if u := r.find(a); u.Ban != 2 {
		t.Fatalf("ban is %d", u.Ban)
	}
	if u := r.find(b); u.Ban != 0 {
		t.Fatal("the other team's red card was applied to my player")
	}
	if lu := r.myLineup(); len(lu.Units) != 1 || lu.Units[0].DefID != "kudus" {
		t.Fatalf("suspended player still in the line-up: %+v", lu.Units)
	}
	// He can be benched, but not brought back while suspended.
	if err := r.Apply(Action{Type: "move", UID: a, Line: -1, Lane: 5}); err != nil {
		t.Fatal(err)
	}
	mustRule(t, r.Apply(Action{Type: "move", UID: a, Line: 2, Lane: 2}))
	mustRule(t, r.Apply(Action{Type: "move", UID: b, Line: -1, Lane: 5})) // swapping him on for a pitch player is blocked too
	if err := r.Apply(Action{Type: "move", UID: c, Line: -1, Lane: 5}); err != nil {
		t.Fatalf("bench players may trade places with him: %v", err)
	}
	r.Discipline(nil, 0)
	mustRule(t, r.Apply(Action{Type: "move", UID: a, Line: 2, Lane: 2}))
	r.Discipline(nil, 0)
	if err := r.Apply(Action{Type: "move", UID: a, Line: 2, Lane: 2}); err != nil {
		t.Fatalf("ban should be over: %v", err)
	}
}
