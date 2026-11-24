package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"footy/internal/game"
	"footy/internal/store"
)

// playDaily plays today's challenge to its end through the API.
func playDaily(t *testing.T, c *http.Client, base string) {
	t.Helper()
	var v game.RunView
	if code := call(t, c, "POST", base+"/api/run?daily=1", nil, &v); code != 201 {
		t.Fatalf("daily: %d", code)
	}
	if v.Daily == "" {
		t.Fatal("the daily run does not say its day")
	}
	if code := call(t, c, "POST", base+"/api/run?daily=1", nil, nil); code != 422 {
		t.Fatalf("a second daily on the same day: %d", code)
	}
	for i := 0; i < 40 && v.Phase != "ended"; i++ {
		if call(t, c, "POST", base+"/api/match/start?daily=1", nil, nil) != 200 {
			// a story event or an augment waits for an answer first
			for _, a := range []map[string]any{{"type": "story", "slot": 0}, {"type": "augment", "slot": 0}} {
				call(t, c, "POST", base+"/api/run/action?daily=1", a, nil)
			}
			continue
		}
		var h halfResponse
		if code := call(t, c, "POST", base+"/api/match/second-half?daily=1", map[string]string{"tactic": "balanced"}, &h); code != 200 {
			t.Fatalf("second half: %d", code)
		}
		v = h.Run
	}
	if v.Phase != "ended" || v.Cup != nil {
		t.Fatalf("the daily did not end with the league: phase %s, cup %v", v.Phase, v.Cup)
	}
}

func TestGuestPlaysThenClaims(t *testing.T) {
	ts, st, c, _ := testServer(t)
	var g struct{ Name string }
	if code := call(t, c, "POST", ts.URL+"/api/auth/guest", map[string]any{}, &g); code != 201 || g.Name == "" {
		t.Fatalf("guest: %d %q", code, g.Name)
	}
	var me struct {
		Name  string
		Guest bool
	}
	call(t, c, "GET", ts.URL+"/api/me", nil, &me)
	if !me.Guest || me.Name != g.Name {
		t.Fatalf("me: %+v", me)
	}
	// A guest has no password: nobody can sign in to one, whatever they type.
	jar0, _ := cookiejar.New(nil)
	for _, pw := range []string{"", "a-long-password-1"} {
		if code := call(t, &http.Client{Jar: jar0}, "POST", ts.URL+"/api/auth/login", map[string]string{"username": g.Name, "password": pw}, nil); code != 401 {
			t.Fatalf("signed in to a guest with %q: %d", pw, code)
		}
	}
	if code := call(t, c, "POST", ts.URL+"/api/run", nil, nil); code != 201 {
		t.Fatalf("a guest season: %d", code)
	}
	for _, p := range []string{"/api/friends/request", "/api/ranked/queue", "/api/series/invite", "/api/party/create", "/api/chat"} {
		if code := call(t, c, "POST", ts.URL+p, map[string]string{}, nil); code != 403 {
			t.Fatalf("a guest reached %s: %d", p, code)
		}
	}
	playDaily(t, c, ts.URL)

	// Somebody else, with a real account, sees the guest in today's table.
	jar, _ := cookiejar.New(nil)
	o := &http.Client{Jar: jar}
	if code := call(t, o, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "rival_1", "email": "rival@game.test", "password": "a-long-password-1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := call(t, o, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "Guest_abc", "email": "g@game.test", "password": "a-long-password-1"}, nil); code != 422 {
		t.Fatalf("a guest-looking name was allowed: %d", code)
	}
	var d struct {
		Status string
		Top    []dailyRow
		Rival  *dailyRow
	}
	call(t, o, "GET", ts.URL+"/api/daily?vs="+g.Name, nil, &d)
	if d.Status != "none" || len(d.Top) != 1 || d.Rival == nil || d.Rival.Name != g.Name {
		t.Fatalf("daily for a rival: %+v", d)
	}

	// The guest claims the account: same id, so the season and the result stay.
	if code := call(t, c, "POST", ts.URL+"/api/auth/claim", map[string]string{"username": "rival_1", "email": "new@game.test", "password": "a-long-password-1"}, nil); code != 409 {
		t.Fatalf("claim with a taken name: %d", code)
	}
	if code := call(t, c, "POST", ts.URL+"/api/auth/claim", map[string]string{"username": "new_boss", "email": "new@game.test", "password": "a-long-password-1"}, nil); code != 200 {
		t.Fatalf("claim: %d", code)
	}
	call(t, c, "GET", ts.URL+"/api/me", nil, &me)
	if me.Guest || me.Name != "new_boss" {
		t.Fatalf("after the claim: %+v", me)
	}
	call(t, c, "GET", ts.URL+"/api/daily", nil, &d)
	if d.Status != "done" || len(d.Top) != 1 || d.Top[0].Name != "new_boss" || !d.Top[0].You {
		t.Fatalf("daily after the claim: %+v", d)
	}
	if code := call(t, c, "POST", ts.URL+"/api/friends/request", map[string]string{"username": "rival_1"}, nil); code >= 300 {
		t.Fatalf("a claimed account cannot add friends: %d", code)
	}
	if code := call(t, c, "POST", ts.URL+"/api/auth/claim", map[string]string{"username": "again_1", "email": "a@game.test", "password": "a-long-password-1"}, nil); code != 409 {
		t.Fatalf("claimed twice: %d", code)
	}
	if code := call(t, c, "POST", ts.URL+"/api/auth/login", map[string]string{"username": "new_boss", "password": "a-long-password-1"}, nil); code != 200 {
		t.Fatalf("login after the claim: %d", code)
	}

	// An unclaimed guest is erased without a trace.
	jar2, _ := cookiejar.New(nil)
	q := &http.Client{Jar: jar2}
	call(t, q, "POST", ts.URL+"/api/auth/guest", map[string]any{}, &g)
	ctx := context.Background()
	due, _ := st.DueForPurge(ctx, time.Now().Add(time.Minute), 10)
	if len(due) != 1 || due[0].Name != g.Name {
		t.Fatalf("due for purge: %+v", due)
	}
	if err := st.PurgeUser(ctx, &due[0], "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UserByID(ctx, due[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("guest still there: %v", err)
	}
}
