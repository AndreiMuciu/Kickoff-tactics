package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"footy/internal/game"
	"footy/internal/store"
)

// Levels: Bronze the first time, higher levels as the count grows; nothing
// is lost or given twice, and dates of achievements from before levels carry over.
func TestAchievementLevels(t *testing.T) {
	st := store.NewMemory()
	srv := &Server{st: st, log: testLog()}
	ctx := context.Background()
	old := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	u := store.User{ID: "u", NameKey: "u", Stats: map[string]int{}, Achieved: map[string]time.Time{"first_win": old, "wins_50": old, "silver": old}}
	if err := st.CreateUser(ctx, &u); err != nil {
		t.Fatal(err)
	}
	level := func(id string) achievementView {
		got, _ := st.UserByID(ctx, "u")
		for _, v := range achievementViews(got) {
			if v.ID == id {
				return v
			}
		}
		t.Fatalf("no achievement %s", id)
		return achievementView{}
	}

	// Old unlocks: Winner keeps Bronze and Gold (50 wins) with their dates;
	// Silver (10 wins) is missing until the counter says so.
	if v := level("first_win"); v.Level != 1 || v.At[0] != old.UnixMilli() {
		t.Fatalf("winner before: %+v", v)
	}
	if v := level("climber"); v.Level != 1 {
		t.Fatalf("climber from the old Silver badge: %+v", v)
	}

	keys := srv.recordStats(ctx, "u", map[string]int{"wins": 55, "bigWins": 1})
	want := map[string]bool{"first_win@2": true, "big_win": true}
	if len(keys) != len(want) {
		t.Fatalf("unlocked %v", keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("unexpected unlock %s in %v", k, keys)
		}
	}
	if v := level("first_win"); v.Level != 3 || v.Next != 150 || v.Count != 55 || len(v.At) != 3 || v.At[2] != old.UnixMilli() {
		t.Fatalf("winner after 55 wins: %+v", v)
	}
	if again := srv.recordStats(ctx, "u", map[string]int{"wins": 1}); len(again) != 0 {
		t.Fatalf("unlocked again: %v", again)
	}
	// Up to Diamond, and no level past it.
	srv.recordStats(ctx, "u", map[string]int{"wins": 1000})
	if v := level("first_win"); v.Level != 5 || v.Next != 0 {
		t.Fatalf("winner at 1056 wins: %+v", v)
	}
	if got := views([]string{"first_win@5", "big_win"}); got[0].Tier != "Diamond" || got[1].Tier != "Bronze" || got[1].Name != "Thrashing" {
		t.Fatalf("views: %+v", got)
	}
}

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// How a run after the league ends feeds the cup and play-off achievements.
func TestCupStats(t *testing.T) {
	for _, c := range []struct {
		cup, out, result string
		want             map[string]int
	}{
		{"crown", "winner", "champion", map[string]int{"cupQuarters": 1, "cupWins": 1, "crownWins": 1, "doubles": 1}},
		{"shield", "winner", "europe", map[string]int{"cupQuarters": 1, "cupWins": 1}},
		{"plate", "semi", "europe", map[string]int{"cupQuarters": 1}},
		{"crown", "group", "champion", map[string]int{}},
		{"playoff", "promoted", "relegated", map[string]int{"promotions": 1}},
		{"playoff", "sacked", "sacked", map[string]int{}},
	} {
		d := map[string]int{}
		cupStats(d, &game.Run{Result: c.result, Cup: &game.Cup{ID: c.cup, Out: c.out}})
		if fmt.Sprint(d) != fmt.Sprint(c.want) {
			t.Errorf("%s %s: %v, want %v", c.cup, c.out, d, c.want)
		}
	}
}
