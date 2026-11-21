package store

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"footy/internal/game"
)

// Runs only against a real MongoDB: MONGODB_TEST_URI=mongodb://127.0.0.1:27017.
// It uses (and drops) the database footy_store_test.
func testMongo(t *testing.T) *Mongo {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI to run against MongoDB")
	}
	ctx := context.Background()
	m, err := NewMongo(ctx, uri, "footy_store_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = m.client.Database("footy_store_test").Drop(ctx)
		_ = m.Close(ctx)
	})
	_ = m.client.Database("footy_store_test").Drop(ctx)
	m, err = NewMongo(ctx, uri, "footy_store_test") // indexes again after the drop
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMongoSeasonsHistoryAchievements(t *testing.T) {
	m := testMongo(t)
	ctx := context.Background()
	tiers := []TierMin{{"Bronze", 0}, {"Silver", 100}, {"Gold", 250}, {"Platinum", 450}, {"Diamond", 700}}
	for _, u := range []User{
		{ID: "a", NameKey: "a", Season: 1},
		{ID: "b", NameKey: "b", LP: 41},
		{ID: "c", NameKey: "c", Season: 2, LP: 90},
	} {
		u := u
		if err := m.CreateUser(ctx, &u); err != nil {
			t.Fatal(err)
		}
	}
	// a plays two ranked lobbies; b's account predates seasons and has stats: null.
	_ = m.AddRating(ctx, "a", 200)
	_ = m.AddRating(ctx, "a", 101)
	if _, err := m.users.UpdateOne(ctx, id("b"), bson.D{{Key: "$set", Value: bson.D{{Key: "achieved", Value: nil}}}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.RollSeason(ctx, 2, tiers); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := m.UserByID(ctx, "a")
	b, _ := m.UserByID(ctx, "b")
	c, _ := m.UserByID(ctx, "c")
	if a.LP != 150 || a.SeasonGames != 0 || a.Season != 2 || len(a.Badges) != 1 || a.Badges[0] != (SeasonBadge{Season: 1, Tier: "Gold", Points: 301}) {
		t.Fatalf("a: %+v", a)
	}
	if b.LP != 20 || len(b.Badges) != 0 || b.Season != 2 {
		t.Fatalf("b: %+v", b)
	}
	if c.LP != 90 || len(c.Badges) != 0 {
		t.Fatalf("c: %+v", c)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := m.SetAchieved(ctx, "b", map[string]time.Time{"first_win": now}); err != nil {
		t.Fatal(err)
	}
	_ = m.SetAchieved(ctx, "b", map[string]time.Time{"top4": now})
	_ = m.SetAchSeen(ctx, "b", now)
	b, _ = m.UserByID(ctx, "b")
	if len(b.Achieved) != 2 || !b.Achieved["first_win"].Equal(now) || !b.AchSeen.Equal(now) {
		t.Fatalf("achievements: %+v seen %v", b.Achieved, b.AchSeen)
	}

	for i := 0; i < 23; i++ {
		g := GameRecord{ID: string(rune('a' + i)), UserID: "a", At: now.Add(time.Duration(i) * time.Minute), Mode: "ranked", Place: i + 1, Matches: []MatchLine{{Round: 1, Opponent: "x", GF: 2, GA: 1, Won: true}}}
		if err := m.AddGame(ctx, &g); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := m.ListGames(ctx, "a", 20, 10)
	if err != nil || total != 23 || len(page) != 3 || page[0].Place != 3 || page[2].Place != 1 || len(page[0].Matches) != 1 {
		t.Fatalf("page 3: %v total %d %+v", err, total, page)
	}
	if err := m.AddFeedback(ctx, &Feedback{ID: "f1", UserID: "a", Name: "a", Kind: "idea", Text: "more replays please", At: now}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoDeleteAndPurge(t *testing.T) {
	m := testMongo(t)
	ctx := context.Background()
	gone := User{ID: "gone1", Name: "gone_me", NameKey: "gone_me", Email: "g@x.test", EmailKey: "g@x.test", PassHash: "h", LP: 50, Stats: map[string]int{"wins": 3}}
	friend := User{ID: "fr1", Name: "friend", NameKey: "friend", LP: 10}
	for _, u := range []*User{&gone, &friend} {
		if err := m.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.PutFriendship(ctx, &Friendship{ID: PairID("gone1", "fr1"), A: "fr1", B: "gone1", Accepted: true})
	_ = m.AddMessage(ctx, &Message{ID: "m1", Pair: PairID("gone1", "fr1"), From: "gone1", Text: "hi", At: time.Now()})
	_ = m.AddFeedback(ctx, &Feedback{ID: "f1", UserID: "gone1", Name: "gone_me", Kind: "idea", Text: "keep me", At: time.Now()})
	_ = m.AddGame(ctx, &GameRecord{ID: "g1", UserID: "fr1", At: time.Now(), Mode: "series", Opponent: "gone_me", Matches: []MatchLine{{Round: 1, Opponent: "gone_me"}, {Round: 2, Opponent: "Imperial XI"}}})
	_ = m.AddGame(ctx, &GameRecord{ID: "g2", UserID: "gone1", At: time.Now(), Mode: "solo"})
	_ = m.SaveRun(ctx, &game.Run{SessionID: "gone1"})
	_ = m.SaveRun(ctx, &game.Run{SessionID: "vs:s1:gone1"})
	_ = m.SaveRun(ctx, &game.Run{SessionID: "vs:s1:fr1"})

	if err := m.SetDeleted(ctx, "gone1", time.Now().Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if top, _ := m.TopRated(ctx, 10); len(top) != 1 || top[0].ID != "fr1" {
		t.Fatalf("ranking: %+v", top)
	}
	if above, total, _ := m.RankInfo(ctx, 0); above != 1 || total != 1 {
		t.Fatalf("rank info: %d of %d", above, total)
	}
	due, err := m.DueForPurge(ctx, time.Now().Add(-30*24*time.Hour), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due: %v %+v", err, due)
	}
	for i := 0; i < 2; i++ {
		if err := m.PurgeUser(ctx, &due[0], "Former manager #abc123"); err != nil {
			t.Fatal(err)
		}
	}
	stub, _ := m.UserByID(ctx, "gone1")
	if stub.Name != "Former manager #abc123" || stub.Email != "" || stub.PassHash != "" || stub.Stats != nil || !stub.Purged {
		t.Fatalf("stub: %+v", stub)
	}
	if _, err := m.UserByName(ctx, "gone_me"); err != ErrNotFound {
		t.Fatalf("old name still resolves: %v", err)
	}
	if due, _ := m.DueForPurge(ctx, time.Now(), 10); len(due) != 0 {
		t.Fatal("a purged account is still due")
	}
	var f Feedback
	_ = m.feedback.FindOne(ctx, id("f1")).Decode(&f)
	if f.Text != "keep me" || f.UserID != "" || f.Name != "" {
		t.Fatalf("feedback: %+v", f)
	}
	games, _, _ := m.ListGames(ctx, "fr1", 0, 10)
	if len(games) != 1 || games[0].Opponent != "Former manager #abc123" || games[0].Matches[0].Opponent != "Former manager #abc123" || games[0].Matches[1].Opponent != "Imperial XI" {
		t.Fatalf("friend's history: %+v", games)
	}
	if mine, total, _ := m.ListGames(ctx, "gone1", 0, 10); total != 0 || len(mine) != 0 {
		t.Fatal("own history survived")
	}
	if fs, _ := m.ListFriendships(ctx, "fr1"); len(fs) != 0 {
		t.Fatal("friendship survived")
	}
	if msgs, _ := m.ListMessages(ctx, PairID("gone1", "fr1"), 10); len(msgs) != 0 {
		t.Fatal("chat survived")
	}
	if _, err := m.GetRun(ctx, "gone1"); err != ErrNotFound {
		t.Fatalf("solo run: %v", err)
	}
	if _, err := m.GetRun(ctx, "vs:s1:gone1"); err != ErrNotFound {
		t.Fatalf("series run: %v", err)
	}
	if _, err := m.GetRun(ctx, "vs:s1:fr1"); err != nil {
		t.Fatalf("the friend's series run went too: %v", err)
	}
	// The name and the address are free again.
	again := User{ID: "new1", Name: "gone_me", NameKey: "gone_me", Email: "g@x.test", EmailKey: "g@x.test"}
	if err := m.CreateUser(ctx, &again); err != nil {
		t.Fatalf("sign-up with the freed name: %v", err)
	}
}
