package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"footy/internal/mail"
	"footy/internal/store"
)

// sentMail keeps the mails sent to one address (mails go out in the background).
type sentMail struct {
	mu  sync.Mutex
	to  string
	got []mail.Message
}

func (m *sentMail) Send(msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg.To == m.to {
		m.got = append(m.got, msg)
	}
	return nil
}
func (m *sentMail) Name() string { return "test" }
func (m *sentMail) list() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message(nil), m.got...)
}

func testServer(t *testing.T) (*httptest.Server, *store.Memory, *http.Client, *sentMail) {
	t.Helper()
	st := store.NewMemory()
	mailer := &sentMail{to: "owner@game.test"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(New(st, Config{AllowedOrigins: []string{"http://game.test"}, Mailer: mailer, FeedbackTo: "owner@game.test"}, log).Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return ts, st, &http.Client{Jar: jar}, mailer
}

func call(t *testing.T, c *http.Client, method, url string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://game.test")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestFeedbackAndAchievement(t *testing.T) {
	ts, _, c, mailer := testServer(t)
	if code := call(t, c, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "fb_user", "email": "fb@game.test", "password": "a-long-password-1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := call(t, c, "POST", ts.URL+"/api/feedback", map[string]string{"kind": "idea", "text": "short", "screen": "home"}, nil); code != 422 {
		t.Fatalf("short feedback: %d", code)
	}
	if code := call(t, c, "POST", ts.URL+"/api/feedback", map[string]string{"kind": "spam", "text": "long enough text here", "screen": "home"}, nil); code != 400 {
		t.Fatalf("unknown kind: %d", code)
	}
	var ok struct {
		Unlocked []achievement `json:"unlocked"`
	}
	text := "Please add a replay button after each match, Ștefan says hi."
	if code := call(t, c, "POST", ts.URL+"/api/feedback", map[string]string{"kind": "idea", "text": text, "screen": "home"}, &ok); code != 200 {
		t.Fatalf("feedback: %d", code)
	}
	if len(ok.Unlocked) != 1 || ok.Unlocked[0].ID != "feedback" {
		t.Fatalf("unlocked: %+v", ok.Unlocked)
	}
	// The mail goes out in the background.
	for i := 0; i < 200 && len(mailer.list()) == 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if got := mailer.list(); len(got) != 1 || !strings.Contains(got[0].Body, text) {
		t.Fatalf("feedback mail: %+v", got)
	}
	// Achievements show as new until they are seen.
	var me struct {
		New int `json:"newAchievements"`
	}
	call(t, c, "GET", ts.URL+"/api/me", nil, &me)
	if me.New != 1 {
		t.Fatalf("new achievements before seen: %d", me.New)
	}
	call(t, c, "POST", ts.URL+"/api/achievements/seen", map[string]string{}, nil)
	call(t, c, "GET", ts.URL+"/api/me", nil, &me)
	if me.New != 0 {
		t.Fatalf("new achievements after seen: %d", me.New)
	}
	var p struct {
		Achievements []achievementView `json:"achievements"`
	}
	call(t, c, "GET", ts.URL+"/api/profile", nil, &p)
	if len(p.Achievements) != len(achievements) {
		t.Fatalf("profile lists %d achievements", len(p.Achievements))
	}
	for _, a := range p.Achievements {
		if (a.ID == "feedback") != (a.At > 0) {
			t.Fatalf("achievement %s unlocked at %d", a.ID, a.At)
		}
	}
}

func TestHistoryPages(t *testing.T) {
	ts, st, c, _ := testServer(t)
	call(t, c, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "hist_user", "email": "h@game.test", "password": "a-long-password-1"}, nil)
	u, err := st.UserByName(context.Background(), "hist_user")
	if err != nil {
		t.Fatal(err)
	}
	type page struct {
		Games []struct {
			Place int `json:"place"`
		} `json:"games"`
		Page, Pages, Total int
	}
	var p page
	if call(t, c, "GET", ts.URL+"/api/history", nil, &p); p.Total != 0 || p.Pages != 1 || len(p.Games) != 0 {
		t.Fatalf("empty history: %+v", p)
	}
	for i := 1; i <= 23; i++ {
		_ = st.AddGame(context.Background(), &store.GameRecord{ID: string(rune('a' + i)), UserID: u.ID, Mode: "normal", Place: i})
	}
	_ = st.AddGame(context.Background(), &store.GameRecord{ID: "other", UserID: "someone-else", Mode: "normal"})
	call(t, c, "GET", ts.URL+"/api/history?page=1", nil, &p)
	if p.Total != 23 || p.Pages != 3 || len(p.Games) != historyPage || p.Games[0].Place != 23 {
		t.Fatalf("page 1: total %d pages %d games %d first %+v", p.Total, p.Pages, len(p.Games), p.Games)
	}
	call(t, c, "GET", ts.URL+"/api/history?page=3", nil, &p)
	if len(p.Games) != 3 || p.Games[2].Place != 1 {
		t.Fatalf("page 3: %+v", p.Games)
	}
	call(t, c, "GET", ts.URL+"/api/history?page=9", nil, &p)
	if len(p.Games) != 0 {
		t.Fatalf("past the end: %+v", p.Games)
	}
}

func TestSeasons(t *testing.T) {
	if n := seasonAt(time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)); n != 1 {
		t.Fatalf("November 2026 is season %d", n)
	}
	if n := seasonAt(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)); n != 4 {
		t.Fatalf("February 2027 is season %d", n)
	}
	if e := seasonEnd(2); !e.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("season 2 ends %v", e)
	}
	st := store.NewMemory()
	ctx := context.Background()
	for _, u := range []store.User{
		{ID: "a", NameKey: "a", LP: 301, Season: 1, SeasonGames: 4},
		{ID: "b", NameKey: "b", LP: 40},                            // older than seasons, never played ranked in it
		{ID: "c", NameKey: "c", LP: 90, Season: 2, SeasonGames: 1}, // already in season 2
	} {
		u := u
		if err := st.CreateUser(ctx, &u); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ { // twice: the reset must not apply twice
		if err := st.RollSeason(ctx, 2, tierMins()); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := st.UserByID(ctx, "a")
	b, _ := st.UserByID(ctx, "b")
	c, _ := st.UserByID(ctx, "c")
	if a.LP != 150 || a.SeasonGames != 0 || len(a.Badges) != 1 || a.Badges[0] != (store.SeasonBadge{Season: 1, Tier: "Gold", Points: 301}) {
		t.Fatalf("a after the reset: %+v", a)
	}
	if b.LP != 20 || len(b.Badges) != 0 || b.Season != 2 {
		t.Fatalf("b after the reset: %+v", b)
	}
	if c.LP != 90 || len(c.Badges) != 0 {
		t.Fatalf("c after the reset: %+v", c)
	}
}

func TestFriendProfile(t *testing.T) {
	ts, _, c1, _ := testServer(t)
	jar, _ := cookiejar.New(nil)
	c2 := &http.Client{Jar: jar}
	call(t, c1, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "prof_one", "email": "one@game.test", "password": "a-long-password-1"}, nil)
	call(t, c2, "POST", ts.URL+"/api/auth/signup", map[string]string{"username": "prof_two", "email": "two@game.test", "password": "a-long-password-1"}, nil)
	if code := call(t, c1, "GET", ts.URL+"/api/profile?name=prof_two", nil, nil); code != 404 {
		t.Fatalf("a stranger's profile: %d", code)
	}
	call(t, c1, "POST", ts.URL+"/api/friends/request", map[string]string{"username": "prof_two"}, nil)
	if code := call(t, c1, "GET", ts.URL+"/api/profile?name=prof_two", nil, nil); code != 404 {
		t.Fatalf("a pending friend's profile: %d", code)
	}
	call(t, c2, "POST", ts.URL+"/api/friends/respond", map[string]any{"username": "prof_one", "accept": true}, nil)
	var p map[string]any
	if code := call(t, c1, "GET", ts.URL+"/api/profile?name=PROF_TWO", nil, &p); code != 200 {
		t.Fatalf("a friend's profile: %d", code)
	}
	if p["name"] != "prof_two" || p["friend"] != true || p["email"] != nil || p["achievements"] == nil {
		t.Fatalf("friend profile: %v", p)
	}
}
