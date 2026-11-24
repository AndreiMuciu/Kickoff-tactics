package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"footy/internal/auth"
	"footy/internal/game"
	"footy/internal/store"
)

// Play now: a guest account is made on the spot, so a visitor reaches the
// pitch without a form. It can play the solo season and the daily challenge;
// everything with other managers waits until it is claimed with a name, an
// email and a password. Unclaimed guests are erased like deleted accounts.

const guestPrefix = "guest_" // reserved: nobody signs up with a name like a guest's

func guestName() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "Guest_" + hex.EncodeToString(b[:]), nil
}

func (s *Server) guest(w http.ResponseWriter, r *http.Request) {
	if !decode(w, r, &struct{}{}) {
		return
	}
	if !s.authIP.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	for try := 0; ; try++ {
		name, err := guestName()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		u := &store.User{ID: id, Name: name, NameKey: strings.ToLower(name), CreatedAt: now, LastSeen: now, Stats: map[string]int{}, Season: seasonAt(now), Guest: true, DeletedAt: now}
		err = s.st.CreateUser(r.Context(), u)
		if errors.Is(err, store.ErrConflict) && try < 3 {
			continue
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.startSession(w, r, id); err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, 201, map[string]string{"name": name})
		return
	}
}

// newAccount checks what a sign-up and a claimed guest both need.
func newAccount(username, email, password string) (mail, mailKey, msg string) {
	mail, mailKey, ok := normEmail(email)
	switch {
	case !ok:
		return "", "", "Enter a valid email address."
	case !nameRE.MatchString(username):
		return "", "", "Username: 3-20 letters, digits or underscores."
	case strings.HasPrefix(strings.ToLower(username), guestPrefix):
		return "", "", "Names starting with Guest_ are kept for guests. Pick another."
	}
	if msg := checkPassword(password, username); msg != "" {
		return "", "", msg
	}
	return mail, mailKey, ""
}

// claim turns the signed-in guest into a normal account, keeping its season,
// statistics, achievements and history.
func (s *Server) claim(w http.ResponseWriter, r *http.Request, uid string) {
	var c struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &c) {
		return
	}
	if !s.authIP.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	email, key, msg := newAccount(c.Username, c.Email, c.Password)
	if msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, msg)
		return
	}
	hash, err := auth.Hash(c.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	switch err := s.st.ClaimGuest(r.Context(), uid, c.Username, strings.ToLower(c.Username), hash, email, key); {
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "That username is taken.")
		return
	case errors.Is(err, store.ErrEmailTaken):
		writeErr(w, http.StatusConflict, "That email address already has an account. Log in to it instead.")
		return
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusConflict, "This account already has a name.")
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	if u, err := s.st.UserByID(r.Context(), uid); err == nil {
		s.sendVerify(r, u)
	}
	writeJSON(w, 200, map[string]string{"name": c.Username})
}

// guestOnly lists the changes a guest may make: its own seasons and little else.
// Anything with other managers (friends, chat, lobbies, series) needs an account.
func guestAllowed(path string) bool {
	switch path {
	case "/api/run", "/api/run/action", "/api/match/start", "/api/match/second-half", "/api/auth/claim",
		"/api/feedback", "/api/feedback/later", "/api/achievements/seen", "/api/plan":
		return true
	}
	return false
}

// ---- daily challenge ----

// dailyKey is where a manager's daily challenge is stored: one per manager,
// replaced by the next day's.
func dailyKey(uid string) string { return "daily:" + uid }

func today() string { return time.Now().UTC().Format("2006-01-02") }

// runOwner is the stored season a run request is about: the manager's own
// season, or his daily challenge with ?daily=1.
func runOwner(r *http.Request, uid string) string {
	if r.URL.Query().Get("daily") == "1" {
		return dailyKey(uid)
	}
	return uid
}

type dailyRow struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Points int    `json:"points"`
	GD     int    `json:"gd"`
	Place  int    `json:"place"`
	Result string `json:"result"`
	You    bool   `json:"you,omitempty"`
}

const dailyShown = 10

// daily is today's challenge: its table and where the manager (and, with
// ?vs=name, the friend who sent the challenge) stands.
func (s *Server) daily(w http.ResponseWriter, r *http.Request, uid string) {
	day := today()
	// shortcut: ranks come from the best 200 results; past that a result shows without a rank. Count with an index when days get bigger.
	recs, err := s.st.DailyTop(r.Context(), day, 200)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(recs))
	for _, g := range recs {
		ids = append(ids, g.UserID)
	}
	users, err := s.st.UsersByIDs(r.Context(), ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vs := strings.ToLower(r.URL.Query().Get("vs"))
	out := map[string]any{"day": day, "endsAt": time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour).UnixMilli(), "players": len(recs)}
	top := []dailyRow{}
	rank := 0
	for _, g := range recs {
		u := users[g.UserID]
		if u == nil || (u.Hidden() && !u.Guest) { // an account being deleted is not shown
			continue
		}
		rank++
		row := dailyRow{Rank: rank, Name: u.Name, Points: g.Points, GD: g.GD, Place: g.Place, Result: g.Result, You: g.UserID == uid}
		if rank <= dailyShown {
			top = append(top, row)
		}
		if row.You {
			out["mine"] = row
		}
		if vs != "" && u.NameKey == vs && !row.You {
			out["rival"] = row
		}
	}
	out["top"] = top
	status := "none"
	if run, err := s.st.GetRun(r.Context(), dailyKey(uid)); err == nil && run.Daily == day {
		status = "playing"
		if run.Phase == "ended" {
			status = "done"
		}
		if run.League != nil {
			out["round"], out["points"] = run.Round, run.League.Mine().Pts
		}
	}
	out["status"] = status
	writeJSON(w, 200, out)
}

// newDaily starts today's challenge, once per day.
func (s *Server) newDaily(r *http.Request, uid string) (*game.Run, error) {
	day := today()
	run := game.NewDailyRun(dailyKey(uid), day)
	old, err := s.st.GetRun(r.Context(), dailyKey(uid))
	switch {
	case err == nil && old.Daily == day:
		return nil, game.ErrRule{Msg: "You have played today's challenge. A new one starts at midnight UTC."}
	case err == nil:
		run.Version = old.Version
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	return run, s.st.SaveRun(r.Context(), run)
}
