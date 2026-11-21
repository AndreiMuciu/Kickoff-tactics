package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"footy/internal/store"
)

func TestDeleteAccountGraceAndPurge(t *testing.T) {
	st := store.NewMemory()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(st, Config{AllowedOrigins: []string{"http://game.test"}, Mailer: &sentMail{}}, log)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	gone, friend := client(), client()
	pw := "a-long-password-1"
	call(t, gone, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "del_me", "email": "del@game.test", "password": pw}, nil)
	call(t, friend, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "del_friend", "email": "fr@game.test", "password": pw}, nil)
	call(t, gone, "POST", ts.URL+"/api/friends/request", map[string]string{"username": "del_friend"}, nil)
	call(t, friend, "POST", ts.URL+"/api/friends/respond", map[string]any{"username": "del_me", "accept": true}, nil)
	call(t, gone, "POST", ts.URL+"/api/feedback", map[string]string{"kind": "idea", "text": "Please keep this idea after I leave.", "screen": "home"}, nil)
	ctx := context.Background()
	me, _ := st.UserByName(ctx, "del_me")
	fr, _ := st.UserByName(ctx, "del_friend")
	_ = st.AddGame(ctx, &store.GameRecord{ID: "g1", UserID: fr.ID, Mode: "series", Opponent: "del_me", Matches: []store.MatchLine{{Round: 1, Opponent: "del_me"}, {Round: 2, Opponent: "Imperial XI"}}})
	_ = st.AddGame(ctx, &store.GameRecord{ID: "g2", UserID: me.ID, Mode: "solo"})

	if code := call(t, gone, "POST", ts.URL+"/api/profile/delete", map[string]string{"password": "wrong-password"}, nil); code != 403 {
		t.Fatalf("delete with a wrong password: %d", code)
	}
	if code := call(t, gone, "POST", ts.URL+"/api/profile/delete", map[string]string{"password": pw}, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	// Hidden at once: signed out, invisible to friends and rankings, name still taken.
	if code := call(t, gone, "GET", ts.URL+"/api/me", nil, nil); code != 401 {
		t.Fatalf("still signed in after deleting: %d", code)
	}
	var fl struct{ Friends []friendView }
	call(t, friend, "GET", ts.URL+"/api/friends", nil, &fl)
	if len(fl.Friends) != 0 {
		t.Fatalf("a deleted friend is still listed: %+v", fl.Friends)
	}
	if code := call(t, friend, "GET", ts.URL+"/api/profile?name=del_me", nil, nil); code != 404 {
		t.Fatalf("a deleted friend's profile: %d", code)
	}
	if top, _ := st.TopRated(ctx, 10); len(top) != 1 {
		t.Fatalf("ranking holds %d managers", len(top))
	}
	if code := call(t, client(), "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "del_me", "email": "x@game.test", "password": pw}, nil); code != 409 {
		t.Fatalf("the name was free during the grace period: %d", code)
	}
	// Signing in restores everything.
	var in struct{ Restored bool }
	if code := call(t, gone, "POST", ts.URL+"/api/auth/login", map[string]string{"username": "del_me", "password": pw}, &in); code != 200 || !in.Restored {
		t.Fatalf("sign-in during grace: %d %+v", code, in)
	}
	call(t, friend, "GET", ts.URL+"/api/friends", nil, &fl)
	if len(fl.Friends) != 1 {
		t.Fatal("the restored friend is missing")
	}

	// Delete again, let the grace period pass, run the janitor.
	call(t, gone, "POST", ts.URL+"/api/profile/delete", map[string]string{"password": pw}, nil)
	_ = st.SetDeleted(ctx, me.ID, time.Now().Add(-deleteGrace-time.Hour))
	if n := srv.purgeDue(ctx); n != 1 {
		t.Fatalf("purged %d accounts", n)
	}
	if n := srv.purgeDue(ctx); n != 0 {
		t.Fatalf("purged again: %d", n)
	}
	anon := anonName(me.ID)
	stub, _ := st.UserByID(ctx, me.ID)
	if stub.Name != anon || stub.Email != "" || stub.PassHash != "" || stub.Stats != nil || !stub.Purged {
		t.Fatalf("stub: %+v", stub)
	}
	if code := call(t, gone, "POST", ts.URL+"/api/auth/login", map[string]string{"username": "del_me", "password": pw}, nil); code != 401 {
		t.Fatalf("sign-in after purge: %d", code)
	}
	games, _, _ := st.ListGames(ctx, me.ID, 0, 10)
	if len(games) != 0 {
		t.Fatal("the deleted manager's history survived")
	}
	games, _, _ = st.ListGames(ctx, fr.ID, 0, 10)
	if g := games[0]; g.Opponent != anon || g.Matches[0].Opponent != anon || g.Matches[1].Opponent != "Imperial XI" {
		t.Fatalf("friend's history: %+v", g)
	}
	if fs, _ := st.ListFriendships(ctx, fr.ID); len(fs) != 0 {
		t.Fatal("the friendship survived")
	}
	// The name is free again.
	if code := call(t, client(), "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "del_me", "email": "del@game.test", "password": pw}, nil); code != 201 {
		t.Fatalf("sign-up with the freed name and address: %d", code)
	}
}
