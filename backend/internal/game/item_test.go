package game

import (
	"fmt"
	"testing"
)

// Items arrive every third round, go on one player each, and come back to
// the bag when he is sold, swapped or merged; keepers take only keeper items.
func TestItems(t *testing.T) {
	r, _ := NewRun("items")
	g := NewRNG(5)
	for r.Round = 1; r.Round <= 9; r.Round++ {
		r.dropItem(g)
	}
	if len(r.Items) != 3 {
		t.Fatalf("%d items after nine rounds, want 3", len(r.Items))
	}
	r.Items = []string{"boots", "gloves", "spikes"}
	r.Units = append(r.Units, Unit{UID: "a", Def: "kane", Stars: 1, Line: 2, Lane: 2}, Unit{UID: "b", Def: "kane", Stars: 1, Line: -1, Lane: 0})
	if err := r.equip(1, "a"); err == nil {
		t.Fatal("gloves went on a striker")
	}
	if err := r.equip(0, "u1"); err == nil {
		t.Fatal("boots went on the keeper")
	}
	if err := r.equip(1, "u1"); err != nil || r.find("u1").Item != "gloves" {
		t.Fatalf("gloves on the keeper: %v", err)
	}
	if err := r.equip(0, "a"); err != nil || r.find("a").Item != "boots" {
		t.Fatalf("boots: %v", err)
	}
	// Equipping another item swaps: the boots go back to the bag.
	if err := r.Apply(Action{Type: "equip", Slot: 0, UID: "a"}); err != nil || r.find("a").Item != "spikes" || fmt.Sprint(r.Items) != "[boots]" {
		t.Fatalf("swap: %v, wearing %q, bag %v", err, r.find("a").Item, r.Items)
	}
	if err := r.Apply(Action{Type: "equip", Slot: 0, UID: "b"}); err != nil {
		t.Fatal(err)
	}
	// A third copy merges them: one item stays on the merged player, one comes back.
	r.Budget, r.Shop = 50, []string{"kane", "", "", "", ""}
	if err := r.Apply(Action{Type: "buy", Slot: 0, Def: "kane"}); err != nil {
		t.Fatal(err)
	}
	var merged *Unit
	for i := range r.Units {
		if r.Units[i].Def == "kane" {
			if merged != nil {
				t.Fatal("no merge")
			}
			merged = &r.Units[i]
		}
	}
	if merged.Stars != 2 || merged.Item == "" || len(r.Items) != 1 {
		t.Fatalf("after the merge: %+v, bag %v", merged, r.Items)
	}
	// Selling gives the item back.
	if err := r.Apply(Action{Type: "sell", UID: merged.UID}); err != nil || len(r.Items) != 2 {
		t.Fatalf("sell: %v, bag %v", err, r.Items)
	}
	if err := r.Apply(Action{Type: "unequip", UID: "u1"}); err != nil || len(r.Items) != 3 || r.find("u1").Item != "" {
		t.Fatalf("unequip: %v, bag %v", err, r.Items)
	}
}

// An item changes the match: the same striker scores more with Golden Boots.
func TestItemsCount(t *testing.T) {
	plain, booted := 0, 0
	for seed := uint64(1); seed <= 1500; seed++ {
		lus := [2]Lineup{weakTeam(), weakTeam()}
		plain += Simulate(seed, lus, balanced, 2).Score[0]
		for i := range lus[0].Units {
			lus[0].Units[i].Item = "boots"
		}
		lus[0].GK.Item = "gloves"
		booted += Simulate(seed, lus, balanced, 2).Score[0]
	}
	if booted <= plain {
		t.Fatalf("goals with boots %d, without %d", booted, plain)
	}
}

// Bots hand out their items to players on the pitch.
func TestBotsUseItems(t *testing.T) {
	r, _ := NewVersusRun("bot-items")
	g := NewRNG(3)
	r.Budget = 40
	r.BotManage(g)
	r.Items = []string{"boots", "gloves"}
	r.BotManage(g)
	if len(r.Items) != 0 {
		t.Fatalf("bot left %v in the bag", r.Items)
	}
	for _, u := range r.Units {
		if u.Item != "" && u.Line < 0 {
			t.Fatalf("an item went to the bench: %+v", u)
		}
	}
}
