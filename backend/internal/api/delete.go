package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	mailer "footy/internal/mail"
)

// Deleting an account has a grace period. The request hides the account at
// once (no sign-in session, not in rankings, friend lists or searches) and
// keeps everything, the name included, so nobody else can take it. Signing in
// within the grace period restores the account as it was. After it, the
// janitor erases the personal data (see store.PurgeUser): email, password,
// history, statistics, friends and chats go; the name in other managers'
// history becomes "Former manager #xxxxxx"; feedback stays without a name.
const deleteGrace = 30 * 24 * time.Hour

// anonName is the name a purged account leaves behind. It cannot be signed up
// with (usernames have no spaces or #), so nobody can pose as it.
func anonName(uid string) string {
	h := sha256.Sum256([]byte("former-manager:" + uid))
	return "Former manager #" + hex.EncodeToString(h[:3])
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &b) {
		return
	}
	u, ok := s.confirmPassword(w, r, uid, b.Password)
	if !ok {
		return
	}
	ctx := r.Context()
	if s.playingLobby(ctx, uid) != nil {
		writeErr(w, http.StatusConflict, "You are still in a lobby. Finish or leave it first.")
		return
	}
	if sr, err := s.st.ActiveSeries(ctx, uid); err == nil {
		if sr.Status == "live" {
			writeErr(w, http.StatusConflict, "You are in a live series. Finish or leave it first.")
			return
		}
		_ = s.st.DeleteSeries(ctx, sr.ID) // an invitation, sent or received, goes with the account
	}
	if s.openParty(r, uid) != nil {
		writeErr(w, http.StatusConflict, "Leave your friends lobby first.")
		return
	}
	s.leaveQueues(r, uid, "")
	now := time.Now().UTC()
	if err := s.st.SetDeleted(ctx, uid, now); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.st.DeleteUserSessions(ctx, uid, "")
	_ = s.st.DeleteTokens(ctx, uid, "verify")
	_ = s.st.DeleteTokens(ctx, uid, "reset")
	purgeAt := now.Add(deleteGrace)
	if u.EmailKey != "" {
		s.send(s.letter(u, mailer.Letter{Subject: "Your account will be deleted",
			Text: "You asked us to delete your Kickoff Tactics account. It is hidden from now on and will be erased for good on " +
				purgeAt.Format("2 January 2006") + ". Changed your mind? Sign in before then and everything is back as it was.",
			Button: "Sign in", URL: s.cfg.PublicURL,
			Note: "If you did not ask for this, sign in and change your password."}))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/api", HttpOnly: true, Secure: s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true, "purgeAt": purgeAt.UnixMilli()})
}

// purgeDue erases every account whose grace period is over.
func (s *Server) purgeDue(ctx context.Context) int {
	users, err := s.st.DueForPurge(ctx, time.Now().Add(-deleteGrace), 100)
	if err != nil {
		s.log.Error("purge lookup failed", "err", err)
		return 0
	}
	n := 0
	for i := range users {
		if err := s.st.PurgeUser(ctx, &users[i], anonName(users[i].ID)); err != nil {
			s.log.Error("purge failed", "err", err)
			continue
		}
		n++
	}
	if n > 0 {
		s.log.Info("deleted accounts erased", "count", n)
	}
	return n
}

// Janitor erases expired deleted accounts now and then every hour, until ctx
// ends. Every server runs it; erasing is safe to repeat.
func (s *Server) Janitor(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		run, cancel := context.WithTimeout(ctx, 2*time.Minute)
		s.purgeDue(run)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
