package game

import (
	"fmt"
	"testing"
)

// Club events come before fixtures 3 and 6 only, never twice in a season,
// and a match never starts with one still waiting.
func TestStoryEventsInSeason(t *testing.T) {
	for k := 0; k < 60; k++ {
		r, err := NewRun(fmt.Sprintf("story-%d", k))
		if err != nil {
			t.Fatal(err)
		}
		g := NewRNG(uint64(k) + 9)
		for r.Phase == "shop" {
			if (r.Event != "") != StoryRounds[r.Round] {
				t.Fatalf("season %d fixture %d: event %q", k, r.Round, r.Event)
			}
			r.BotManage(g)
			if _, err := r.StartMatch(); err != nil {
				t.Fatal(err)
			}
			if r.Event != "" {
				t.Fatalf("season %d: the match started with event %q unanswered", k, r.Event)
			}
			if _, _, err := r.FinishMatch("balanced"); err != nil {
				t.Fatal(err)
			}
		}
		seen := map[string]bool{}
		for _, id := range r.EventsSeen {
			if seen[id] {
				t.Fatalf("season %d saw %q twice", k, id)
			}
			seen[id] = true
		}
		if r.Result != "sacked" && len(r.EventsSeen) != len(StoryRounds) {
			t.Fatalf("season %d finished with events %v", k, r.EventsSeen)
		}
	}
}

func TestStoryChoices(t *testing.T) {
	fresh := func(event string) *Run {
		r, _ := NewRun("choices")
		r.Event, r.Budget, r.Patience = event, 2, 10
		return r
	}
	r := fresh("camp")
	if err := r.Apply(Action{Type: "story", Slot: 0}); err == nil {
		t.Fatal("the camp was booked without the budget for it")
	}
	if err := r.Apply(Action{Type: "story", Slot: 1}); err != nil || r.Event != "" {
		t.Fatalf("training at home: %v, event %q", err, r.Event)
	}
	if err := r.Apply(Action{Type: "story", Slot: 0}); err == nil {
		t.Fatal("an event was answered twice")
	}
	r = fresh("sponsor")
	_ = r.Apply(Action{Type: "story", Slot: 0})
	if r.Budget != 2+sponsorCash || r.Patience != 9 {
		t.Fatalf("sponsor: budget %d patience %d", r.Budget, r.Patience)
	}

	// Promises are judged by the next match.
	r = fresh("press")
	_ = r.Apply(Action{Type: "story", Slot: 0})
	if extra, note := r.settlePledge(true, false, 2, 1); extra != 0 || r.Patience != 10+pledgeWinBonus || note == "" {
		t.Fatalf("kept promise: extra %d patience %d note %q", extra, r.Patience, note)
	}
	r = fresh("derby")
	_ = r.Apply(Action{Type: "story", Slot: 0})
	if extra, _ := r.settlePledge(false, false, 0, 1); extra != pledgeBroken || r.Pledge != "" {
		t.Fatalf("broken derby promise: extra %d pledge %q", extra, r.Pledge)
	}
	r = fresh("owner")
	_ = r.Apply(Action{Type: "story", Slot: 0})
	if r.settlePledge(false, true, 0, 0); r.Budget != 2+cleanBonus {
		t.Fatalf("clean sheet bonus: budget %d", r.Budget)
	}
}
