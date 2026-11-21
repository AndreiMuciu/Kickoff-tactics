package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"footy/internal/auth"
	"footy/internal/game"
	"footy/internal/store"
)

const (
	minPassword = 8
	maxPassword = 128
	maxMessage  = 500
	maxFriends  = 100
	chatPage    = 60
	onlineFor   = 75 * time.Second
)

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ---- accounts ----

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// startSession issues a fresh token. A new token on every sign-in means a
// token planted before login (session fixation) is worthless afterwards.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	var b [tokenLength]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	tok := base64.RawURLEncoding.EncodeToString(b[:])
	if err := s.st.CreateSession(r.Context(), hashToken(tok), userID); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/api", HttpOnly: true, Secure: s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(store.SessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
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
	email, emailKey, okEmail := normEmail(c.Email)
	if !okEmail {
		writeErr(w, http.StatusUnprocessableEntity, "Enter a valid email address.")
		return
	}
	if !nameRE.MatchString(c.Username) {
		writeErr(w, http.StatusUnprocessableEntity, "Username: 3-20 letters, digits or underscores.")
		return
	}
	if n := utf8.RuneCountInString(c.Password); n < minPassword || len(c.Password) > maxPassword {
		writeErr(w, http.StatusUnprocessableEntity, "Password: at least 8 characters.")
		return
	}
	if strings.EqualFold(c.Password, c.Username) {
		writeErr(w, http.StatusUnprocessableEntity, "Password must differ from the username.")
		return
	}
	hash, err := auth.Hash(c.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	u := &store.User{ID: id, Name: c.Username, NameKey: strings.ToLower(c.Username), PassHash: hash, CreatedAt: now, LastSeen: now, Email: email, EmailKey: emailKey, Stats: map[string]int{}, Season: seasonAt(now)}
	if err := s.st.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "That username is taken.")
			return
		}
		if errors.Is(err, store.ErrEmailTaken) {
			writeErr(w, http.StatusConflict, "That email address already has an account. Use \"Forgot password\" if it is yours.")
			return
		}
		s.fail(w, r, err)
		return
	}
	s.sendVerify(r, u)
	if err := s.startSession(w, r, u.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]string{"name": u.Name})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	key := strings.ToLower(c.Username)
	// Two throttles: per IP, and per account so a botnet cannot grind one user.
	if !s.authIP.allow(clientIP(r)) || (nameRE.MatchString(c.Username) && !s.authAcc.allow(key)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	const wrong = "Wrong username or password."
	if !nameRE.MatchString(c.Username) || len(c.Password) > maxPassword {
		writeErr(w, http.StatusUnauthorized, wrong)
		return
	}
	u, err := s.st.UserByName(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Same work and same answer as a wrong password: no user enumeration.
			auth.VerifyDummy(c.Password)
			writeErr(w, http.StatusUnauthorized, wrong)
			return
		}
		s.fail(w, r, err)
		return
	}
	if !auth.Verify(c.Password, u.PassHash) {
		writeErr(w, http.StatusUnauthorized, wrong)
		return
	}
	// Signing in during the grace period cancels the deletion.
	restored := false
	if u.Hidden() {
		if err := s.st.SetDeleted(r.Context(), u.ID, time.Time{}); err != nil {
			s.fail(w, r, err)
			return
		}
		restored = true
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"name": u.Name, "restored": restored})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.st.DeleteSession(r.Context(), hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/api", HttpOnly: true, Secure: s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type seasonSummary struct {
	Phase     string `json:"phase"`
	Round     int    `json:"round"`
	MaxRounds int    `json:"maxRounds"`
	Wins      int    `json:"wins"`
	Losses    int    `json:"losses"`
	Patience  int    `json:"patience"`
	Result    string `json:"result"`
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	out := map[string]any{"name": u.Name, "season": nil, "hasSquad": false, "hasEmail": u.EmailKey != "", "verified": u.Verified, "newAchievements": unseenAchievements(u)}
	if run, err := s.loadRun(r.Context(), uid); err == nil {
		_, out["hasSquad"] = run.Squad(u.Name)
		out["season"] = seasonSummary{Phase: run.Phase, Round: run.Round, MaxRounds: game.MaxRounds, Wins: run.Wins, Losses: run.Losses, Patience: run.Patience, Result: run.Result}
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, out)
}

// ---- friends ----

type nameBody struct {
	Username string `json:"username"`
}

func other(f store.Friendship, uid string) string {
	if f.A == uid {
		return f.B
	}
	return f.A
}

// lookup resolves a username from a request body to a user other than the caller.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request, uid, name string) (*store.User, bool) {
	if !nameRE.MatchString(name) {
		writeErr(w, http.StatusUnprocessableEntity, "Invalid username.")
		return nil, false
	}
	u, err := s.st.UserByName(r.Context(), strings.ToLower(name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "No manager with that name.")
			return nil, false
		}
		s.fail(w, r, err)
		return nil, false
	}
	if u.ID == uid {
		writeErr(w, http.StatusUnprocessableEntity, "That is you.")
		return nil, false
	}
	if u.Hidden() { // a deleted account is not there for anybody
		writeErr(w, http.StatusNotFound, "No manager with that name.")
		return nil, false
	}
	return u, true
}

// friendOf resolves a username and requires an accepted friendship with the
// caller. Chat, live series and lobby invites are only possible between friends.
func (s *Server) friendOf(w http.ResponseWriter, r *http.Request, uid, name string) (*store.User, bool) {
	u, ok := s.lookup(w, r, uid, name)
	if !ok {
		return nil, false
	}
	f, err := s.st.GetFriendship(r.Context(), store.PairID(uid, u.ID))
	if err != nil || !f.Accepted {
		writeErr(w, http.StatusForbidden, "You are not friends with that manager.")
		return nil, false
	}
	return u, true
}

type friendView struct {
	Name     string `json:"name"`
	Online   bool   `json:"online"`
	HasSquad bool   `json:"hasSquad"`
}

func (s *Server) friends(w http.ResponseWriter, r *http.Request, uid string) {
	list, err := s.st.ListFriendships(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(list))
	for _, f := range list {
		ids = append(ids, other(f, uid))
	}
	users, err := s.st.UsersByIDs(r.Context(), ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	friends, incoming, outgoing := []friendView{}, []string{}, []string{}
	for _, f := range list {
		u := users[other(f, uid)]
		if u == nil || u.Hidden() {
			continue
		}
		switch {
		case f.Accepted:
			v := friendView{Name: u.Name, Online: time.Since(u.LastSeen) < onlineFor}
			if run, err := s.loadRun(r.Context(), u.ID); err == nil {
				_, v.HasSquad = run.Squad(u.Name)
			}
			friends = append(friends, v)
		case f.Requester == uid:
			outgoing = append(outgoing, u.Name)
		default:
			incoming = append(incoming, u.Name)
		}
	}
	writeJSON(w, 200, map[string]any{"friends": friends, "incoming": incoming, "outgoing": outgoing})
}

func (s *Server) friendRequest(w http.ResponseWriter, r *http.Request, uid string) {
	var b nameBody
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.lookup(w, r, uid, b.Username)
	if !ok {
		return
	}
	id := store.PairID(uid, u.ID)
	if f, err := s.st.GetFriendship(r.Context(), id); err == nil {
		switch {
		case f.Accepted:
			writeErr(w, http.StatusConflict, "You are already friends.")
		case f.Requester == uid:
			writeErr(w, http.StatusConflict, "Invite already sent.")
		default:
			// They invited first: asking back accepts.
			f.Accepted = true
			if err := s.st.PutFriendship(r.Context(), f); err != nil {
				s.fail(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]string{"status": "accepted"})
		}
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	mine, err := s.st.ListFriendships(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(mine) >= maxFriends {
		writeErr(w, http.StatusUnprocessableEntity, "Friend list is full.")
		return
	}
	a, c := uid, u.ID
	if a > c {
		a, c = c, a
	}
	f := &store.Friendship{ID: id, A: a, B: c, Requester: uid, CreatedAt: time.Now().UTC()}
	if err := s.st.PutFriendship(r.Context(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]string{"status": "sent"})
}

func (s *Server) friendRespond(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Username string `json:"username"`
		Accept   bool   `json:"accept"`
	}
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.lookup(w, r, uid, b.Username)
	if !ok {
		return
	}
	f, err := s.st.GetFriendship(r.Context(), store.PairID(uid, u.ID))
	// Only the invited side may answer, and only while the invite is pending.
	if err != nil || f.Accepted || f.Requester == uid {
		writeErr(w, http.StatusNotFound, "No pending invite from that manager.")
		return
	}
	if b.Accept {
		f.Accepted = true
		err = s.st.PutFriendship(r.Context(), f)
	} else {
		err = s.st.DeleteFriendship(r.Context(), f.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) friendRemove(w http.ResponseWriter, r *http.Request, uid string) {
	var b nameBody
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.lookup(w, r, uid, b.Username)
	if !ok {
		return
	}
	if err := s.st.DeleteFriendship(r.Context(), store.PairID(uid, u.ID)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- chat ----

type messageView struct {
	ID     string    `json:"id"`
	Mine   bool      `json:"mine"`
	System bool      `json:"system"`
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
}

func (s *Server) chatList(w http.ResponseWriter, r *http.Request, uid string) {
	u, ok := s.friendOf(w, r, uid, r.URL.Query().Get("with"))
	if !ok {
		return
	}
	list, err := s.st.ListMessages(r.Context(), store.PairID(uid, u.ID), chatPage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]messageView, 0, len(list))
	for _, m := range list {
		out = append(out, messageView{ID: m.ID, Mine: m.From == uid, System: m.System, Text: m.Text, At: m.At})
	}
	writeJSON(w, 200, map[string]any{"messages": out})
}

// cleanMessage trims the text and rejects control characters. The client
// renders messages as plain text, so no markup is ever interpreted.
func cleanMessage(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if t == "" || utf8.RuneCountInString(t) > maxMessage || !utf8.ValidString(t) {
		return "", false
	}
	for _, c := range t {
		if unicode.IsControl(c) && c != '\n' {
			return "", false
		}
	}
	return t, true
}

func (s *Server) chatSend(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.friendOf(w, r, uid, b.To)
	if !ok {
		return
	}
	text, ok := cleanMessage(b.Text)
	if !ok {
		writeErr(w, http.StatusUnprocessableEntity, "Message must be 1-500 characters.")
		return
	}
	if !s.chat.allow(uid) {
		writeErr(w, http.StatusTooManyRequests, "Slow down a little.")
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m := &store.Message{ID: id, Pair: store.PairID(uid, u.ID), From: uid, Text: text, At: time.Now().UTC()}
	if err := s.st.AddMessage(r.Context(), m); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, messageView{ID: m.ID, Mine: true, Text: m.Text, At: m.At})
}
