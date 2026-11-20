package api

import (
	"testing"
	"time"

	"footy/internal/store"
)

func TestRankedFillsWithBotsAfterTwoMinutes(t *testing.T) {
	now := time.Now()
	q := []store.QueueEntry{
		{UserID: "far", Rating: 900, JoinedAt: now.Add(-30 * time.Second)},
		{UserID: "first", Rating: 100, JoinedAt: now.Add(-90 * time.Second)},
		{UserID: "near", Rating: 120, JoinedAt: now.Add(-10 * time.Second)},
	}
	if pickRanked(q, now) != nil || fillRanked(q, now) != nil {
		t.Fatal("three managers started a ranked lobby before anyone waited two minutes")
	}
	later := now.Add(31 * time.Second) // "first" has now waited 2:01
	got := fillRanked(q, later)
	if len(got) != 3 || q[got[0]].UserID != "first" || q[got[1]].UserID != "near" || q[got[2]].UserID != "far" {
		t.Fatalf("picked %v", got)
	}
	// With more than eight waiting, the seven closest in points join the first.
	for i := 0; i < 9; i++ {
		q = append(q, store.QueueEntry{UserID: "x", Rating: 105 + i, JoinedAt: now})
	}
	got = fillRanked(q, later)
	if len(got) != lobbySize || q[got[0]].UserID != "first" {
		t.Fatalf("picked %d, first %s", len(got), q[got[0]].UserID)
	}
	for _, k := range got {
		if q[k].UserID == "far" {
			t.Fatal("the manager furthest in points took a seat over closer ones")
		}
	}
}
