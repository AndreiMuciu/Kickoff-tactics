package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"footy/internal/auth"
	mailer "footy/internal/mail"
	"footy/internal/store"
)

const (
	verifyTTL = 24 * time.Hour
	resetTTL  = 30 * time.Minute
	maxEmail  = 254
)

// normEmail validates an address and returns it with its lower-case key.
func normEmail(v string) (email, key string, ok bool) {
	v = strings.TrimSpace(v)
	if len(v) < 6 || len(v) > maxEmail || strings.ContainsAny(v, " \r\n\t<>,;\"") {
		return "", "", false
	}
	a, err := mail.ParseAddress(v)
	if err != nil || a.Address != v {
		return "", "", false
	}
	at := strings.LastIndex(v, "@")
	if at < 1 || !strings.Contains(v[at+1:], ".") || strings.HasSuffix(v, ".") {
		return "", "", false
	}
	return v, strings.ToLower(v), true
}

func checkPassword(pw, username string) string {
	if n := utf8.RuneCountInString(pw); n < minPassword || len(pw) > maxPassword {
		return "Password: at least 8 characters."
	}
	if strings.EqualFold(pw, username) {
		return "Password must differ from the username."
	}
	return ""
}

// mailToken creates a single-use token of a kind and returns the secret that goes in the link.
func (s *Server) mailToken(r *http.Request, u *store.User, kind string, ttl time.Duration) (string, error) {
	var b [tokenLength]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(b[:])
	// Only the newest link of a kind works.
	_ = s.st.DeleteTokens(r.Context(), u.ID, kind)
	err := s.st.PutToken(r.Context(), &store.Token{ID: hashToken(tok), UserID: u.ID, Kind: kind, EmailKey: u.EmailKey, Expires: time.Now().UTC().Add(ttl)})
	return tok, err
}

// send mails in the background: the request neither waits for the mail server
// nor reveals, through its timing, whether an address has an account.
func (s *Server) send(m mailer.Message) {
	go func() {
		if err := s.cfg.Mailer.Send(m); err != nil {
			s.log.Error("mail failed", "err", err)
		}
	}()
}

// The secret travels in the URL fragment, which browsers never send to a
// server, so it stays out of access logs and Referer headers.
// letter fills in what every account email shares: who it goes to and where the game lives.
func (s *Server) letter(u *store.User, l mailer.Letter) mailer.Message {
	l.To, l.Name, l.Site = u.Email, u.Name, s.cfg.PublicURL
	return l.Message()
}

func (s *Server) link(kind, tok string) string {
	return strings.TrimRight(s.cfg.PublicURL, "/") + "/#" + kind + "=" + tok
}

func (s *Server) sendVerify(r *http.Request, u *store.User) {
	if u.EmailKey == "" || !s.mailTo.allow(u.EmailKey) {
		return
	}
	tok, err := s.mailToken(r, u, "verify", verifyTTL)
	if err != nil {
		s.log.Error("verify token failed", "err", err)
		return
	}
	s.send(s.letter(u, mailer.Letter{Subject: "Confirm your email address",
		Text:   "Welcome to the dugout. Confirm this address to finish setting up your manager account.",
		Button: "Confirm my address", URL: s.link("verify", tok),
		Note: "The link works once and expires in 24 hours. If you did not create this account, ignore this message."}))
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !s.authIP.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	const bad = "This link is no longer valid. Ask for a new one from your profile."
	t, err := s.st.TakeToken(r.Context(), hashToken(b.Token))
	if err != nil || t.Kind != "verify" {
		writeErr(w, http.StatusUnprocessableEntity, bad)
		return
	}
	// Fails when the account's address changed after the mail was sent.
	if err := s.st.SetVerified(r.Context(), t.UserID, t.EmailKey); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, bad)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) forgot(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !s.authIP.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	// The same answer whether or not the address has an account: no enumeration.
	defer writeJSON(w, 200, map[string]bool{"ok": true})
	_, key, ok := normEmail(b.Email)
	if !ok || !s.mailTo.allow(key) {
		return
	}
	u, err := s.st.UserByEmail(r.Context(), key)
	if err != nil {
		return
	}
	tok, err := s.mailToken(r, u, "reset", resetTTL)
	if err != nil {
		s.log.Error("reset token failed", "err", err)
		return
	}
	s.send(s.letter(u, mailer.Letter{Subject: "Reset your password",
		Text:   "Someone asked for a new password for your manager account. Choose one with the button below.",
		Button: "Choose a new password", URL: s.link("reset", tok),
		Note: "The link works once and expires in 30 minutes. If you did not ask for it, ignore this message: your password stays as it is."}))
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !s.authIP.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return
	}
	// Validate the password before spending the single-use token.
	if msg := checkPassword(b.Password, ""); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, msg)
		return
	}
	const bad = "This link is no longer valid. Ask for a new one."
	t, err := s.st.TakeToken(r.Context(), hashToken(b.Token))
	if err != nil || t.Kind != "reset" {
		writeErr(w, http.StatusUnprocessableEntity, bad)
		return
	}
	u, err := s.st.UserByID(r.Context(), t.UserID)
	if err != nil || u.EmailKey != t.EmailKey {
		writeErr(w, http.StatusUnprocessableEntity, bad)
		return
	}
	if strings.EqualFold(b.Password, u.Name) {
		writeErr(w, http.StatusUnprocessableEntity, "Password must differ from the username. Ask for a new link.")
		return
	}
	hash, err := auth.Hash(b.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SetPassword(r.Context(), u.ID, hash); err != nil {
		s.fail(w, r, err)
		return
	}
	// Whoever knew the old password is signed out everywhere. Opening the
	// link also proves the address is the manager's.
	_ = s.st.DeleteUserSessions(r.Context(), u.ID, "")
	_ = s.st.SetVerified(r.Context(), u.ID, u.EmailKey)
	writeJSON(w, 200, map[string]string{"name": u.Name})
}

// ---- profile ----

func (s *Server) profile(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	stats := map[string]int{}
	for k, v := range u.Stats {
		stats[k] = v
	}
	rating := max(0, baseRating+u.LP)
	writeJSON(w, 200, map[string]any{
		"name": u.Name, "email": u.Email, "verified": u.Verified, "since": u.CreatedAt.UnixMilli(),
		"points": rating, "tier": tierOf(rating), "rankedLobbies": u.Games, "stats": stats,
	})
}

func (s *Server) currentSession(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return hashToken(c.Value)
	}
	return ""
}

// confirmPassword re-checks the password before a sensitive change, throttled per account.
func (s *Server) confirmPassword(w http.ResponseWriter, r *http.Request, uid, pw string) (*store.User, bool) {
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return nil, false
	}
	if !s.authAcc.allow(u.NameKey) {
		writeErr(w, http.StatusTooManyRequests, "Too many attempts. Try again in a minute.")
		return nil, false
	}
	if len(pw) > maxPassword || !auth.Verify(pw, u.PassHash) {
		writeErr(w, http.StatusForbidden, "Current password is wrong.")
		return nil, false
	}
	return u, true
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.confirmPassword(w, r, uid, b.Current)
	if !ok {
		return
	}
	if msg := checkPassword(b.New, u.Name); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if b.New == b.Current {
		writeErr(w, http.StatusUnprocessableEntity, "The new password is the same as the current one.")
		return
	}
	hash, err := auth.Hash(b.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SetPassword(r.Context(), uid, hash); err != nil {
		s.fail(w, r, err)
		return
	}
	// Other devices are signed out; this one stays in. Pending reset links die too.
	_ = s.st.DeleteUserSessions(r.Context(), uid, s.currentSession(r))
	_ = s.st.DeleteTokens(r.Context(), uid, "reset")
	if u.Verified {
		s.send(s.letter(u, mailer.Letter{Subject: "Your password was changed",
			Text: "The password of your manager account was just changed from your profile.",
			Note: "If that was not you, reset it now with \"Forgot password\" on the sign-in page."}))
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) changeEmail(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &b) {
		return
	}
	email, key, valid := normEmail(b.Email)
	if !valid {
		writeErr(w, http.StatusUnprocessableEntity, "That does not look like an email address.")
		return
	}
	u, ok := s.confirmPassword(w, r, uid, b.Password)
	if !ok {
		return
	}
	if key == u.EmailKey {
		writeErr(w, http.StatusUnprocessableEntity, "That is already your address.")
		return
	}
	if err := s.st.SetEmail(r.Context(), uid, email, key); err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			writeErr(w, http.StatusConflict, "That address is already used by another account.")
			return
		}
		s.fail(w, r, err)
		return
	}
	// Links mailed to the old address stop working.
	_ = s.st.DeleteTokens(r.Context(), uid, "reset")
	if u.Verified {
		s.send(s.letter(u, mailer.Letter{Subject: "Your email address was changed",
			Text: "The email address of your manager account was changed to another one.",
			Note: "If that was not you, sign in and change your password."}))
	}
	u.Email, u.EmailKey = email, key
	s.sendVerify(r, u)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) resendVerify(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	switch {
	case u.EmailKey == "":
		writeErr(w, http.StatusUnprocessableEntity, "Add an email address first.")
	case u.Verified:
		writeErr(w, http.StatusConflict, "Your address is already confirmed.")
	case !s.mailTo.peek(u.EmailKey):
		writeErr(w, http.StatusTooManyRequests, "A mail was sent a moment ago. Check your inbox, or try again in a few minutes.")
	default:
		s.sendVerify(r, u)
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
