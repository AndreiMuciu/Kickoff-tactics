package api

import (
	"errors"
	"net/http"
	"time"

	"footy/internal/game"
	"footy/internal/store"
)

// A friends lobby is a private lobby: one manager creates it, invites friends,
// and starts it with whoever has joined (two to eight). No bots, no rating.
// While it gathers it is a Lobby document with status "open".
const partyTTL = 15 * time.Minute

type partyView struct {
	ID      string   `json:"id"`
	Host    string   `json:"host"`
	IsHost  bool     `json:"isHost"`
	Members []string `json:"members"`
	Invited []string `json:"invited"`
}

type partyInvite struct {
	ID   string `json:"id"`
	Host string `json:"host"`
}

// openLobby returns the gathering friends lobby the user is in or invited to.
func (s *Server) openLobby(r *http.Request, uid string) *store.Lobby {
	lb, err := s.st.ActiveLobby(r.Context(), uid)
	if err != nil || lb.Status != "open" {
		return nil
	}
	if time.Since(lb.CreatedAt) > partyTTL {
		lb.Status = "done"
		_ = s.st.SaveLobby(r.Context(), lb)
		return nil
	}
	return lb
}

// openParty is the friends lobby the user has actually joined.
func (s *Server) openParty(r *http.Request, uid string) *store.Lobby {
	if lb := s.openLobby(r, uid); lb != nil && seatOf(lb, uid) >= 0 {
		return lb
	}
	return nil
}

func (s *Server) partyState(r *http.Request, uid string) (*partyView, []partyInvite) {
	invites := []partyInvite{}
	lb := s.openLobby(r, uid)
	if lb == nil {
		return nil, invites
	}
	host := ""
	for _, st := range lb.Seats {
		if st.UserID == lb.Host {
			host = st.Name
		}
	}
	if seatOf(lb, uid) < 0 {
		return nil, append(invites, partyInvite{ID: lb.ID, Host: host})
	}
	v := &partyView{ID: lb.ID, Host: host, IsHost: lb.Host == uid, Members: []string{}, Invited: []string{}}
	for _, st := range lb.Seats {
		v.Members = append(v.Members, st.Name)
	}
	if users, err := s.st.UsersByIDs(r.Context(), lb.Invited); err == nil {
		for _, id := range lb.Invited {
			if u := users[id]; u != nil {
				v.Invited = append(v.Invited, u.Name)
			}
		}
	}
	return v, invites
}

// busy reports why a manager cannot start or join another live game right now.
func (s *Server) busy(r *http.Request, uid string) string {
	switch {
	case s.playingLobby(r.Context(), uid) != nil:
		return "You are already in a lobby."
	case s.activeSeries(r, uid) != nil:
		return "Finish or forfeit your live series first."
	}
	return ""
}

func (s *Server) partyCreate(w http.ResponseWriter, r *http.Request, uid string) {
	if msg := s.busy(r, uid); msg != "" {
		writeErr(w, http.StatusConflict, msg)
		return
	}
	if s.openParty(r, uid) != nil {
		writeErr(w, http.StatusConflict, "You already have a friends lobby open.")
		return
	}
	me, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.leaveQueues(r, uid, "")
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lb := &store.Lobby{ID: id, Status: "open", Host: uid, Users: []string{uid}, CreatedAt: time.Now().UTC(),
		Seats: []store.LobbySeat{{UserID: uid, Name: me.Name, Alive: true, Patience: game.VersusPatience}}}
	if err := s.st.SaveLobby(r.Context(), lb); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

// editParty loads the caller's open friends lobby, applies `f` and saves,
// retrying when another member changed it at the same moment.
func (s *Server) editParty(w http.ResponseWriter, r *http.Request, id string, f func(lb *store.Lobby) (int, string)) {
	for try := 0; try < 4; try++ {
		lb, err := s.st.GetLobby(r.Context(), id)
		if err != nil || lb.Status != "open" {
			writeErr(w, http.StatusNotFound, "That friends lobby is no longer open.")
			return
		}
		if code, msg := f(lb); msg != "" {
			writeErr(w, code, msg)
			return
		}
		err = s.st.SaveLobby(r.Context(), lb)
		if err == nil {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if !errors.Is(err, store.ErrConflict) {
			s.fail(w, r, err)
			return
		}
	}
	writeErr(w, http.StatusConflict, "Try again.")
}

func without(list []string, x string) []string {
	out := list[:0:0]
	for _, v := range list {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

func (s *Server) partyInvite(w http.ResponseWriter, r *http.Request, uid string) {
	var b nameBody
	if !decode(w, r, &b) {
		return
	}
	// Anyone already in the lobby may invite, but only his own friends.
	// Starting and closing the lobby stay with the host.
	friend, ok := s.friendOf(w, r, uid, b.Username)
	if !ok {
		return
	}
	mine := s.openParty(r, uid)
	if mine == nil {
		writeErr(w, http.StatusNotFound, "Create a friends lobby first.")
		return
	}
	s.editParty(w, r, mine.ID, func(lb *store.Lobby) (int, string) {
		switch {
		case seatOf(lb, uid) < 0:
			return http.StatusForbidden, "Join the lobby before inviting."
		case seatOf(lb, friend.ID) >= 0:
			return http.StatusConflict, friend.Name + " is already in the lobby."
		case len(lb.Seats)+len(lb.Invited) >= lobbySize:
			return http.StatusConflict, "The lobby is full."
		}
		lb.Invited = append(without(lb.Invited, friend.ID), friend.ID)
		return 0, ""
	})
}

func (s *Server) partyJoin(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID     string `json:"id"`
		Accept bool   `json:"accept"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Accept {
		if msg := s.busy(r, uid); msg != "" {
			writeErr(w, http.StatusConflict, msg)
			return
		}
	}
	me, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.editParty(w, r, b.ID, func(lb *store.Lobby) (int, string) {
		invited := false
		for _, id := range lb.Invited {
			invited = invited || id == uid
		}
		if !invited {
			return http.StatusForbidden, "You are not invited to that lobby."
		}
		lb.Invited = without(lb.Invited, uid)
		if !b.Accept {
			return 0, ""
		}
		if len(lb.Seats) >= lobbySize {
			return http.StatusConflict, "The lobby is full."
		}
		lb.Users = append(lb.Users, uid)
		lb.Seats = append(lb.Seats, store.LobbySeat{UserID: uid, Name: me.Name, Alive: true, Patience: game.VersusPatience})
		return 0, ""
	})
	if b.Accept {
		s.leaveQueues(r, uid, "")
	}
}

func (s *Server) partyLeave(w http.ResponseWriter, r *http.Request, uid string) {
	mine := s.openParty(r, uid)
	if mine == nil {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	s.editParty(w, r, mine.ID, func(lb *store.Lobby) (int, string) {
		if lb.Host == uid {
			lb.Status = "done" // the host leaving closes the lobby for everyone
			return 0, ""
		}
		seats := lb.Seats[:0:0]
		for _, st := range lb.Seats {
			if st.UserID != uid {
				seats = append(seats, st)
			}
		}
		lb.Seats, lb.Users = seats, without(lb.Users, uid)
		return 0, ""
	})
}

// partyStart begins the game with whoever has joined: at least two managers.
func (s *Server) partyStart(w http.ResponseWriter, r *http.Request, uid string) {
	mine := s.openParty(r, uid)
	if mine == nil {
		writeErr(w, http.StatusNotFound, "Create a friends lobby first.")
		return
	}
	lb, err := s.st.GetLobby(r.Context(), mine.ID)
	if err != nil || lb.Status != "open" {
		writeErr(w, http.StatusNotFound, "That friends lobby is no longer open.")
		return
	}
	switch {
	case lb.Host != uid:
		writeErr(w, http.StatusForbidden, "Only the host can start the lobby.")
		return
	case len(lb.Seats) < 2:
		writeErr(w, http.StatusUnprocessableEntity, "You need at least one friend in the lobby to start.")
		return
	}
	// Clubs first, then the lobby goes live: a failed save leaves it open.
	for i := range lb.Seats {
		run, err := game.NewVersusRun(lobbyRunKey(lb.ID, i))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.st.SaveRun(r.Context(), run); err != nil && !errors.Is(err, store.ErrConflict) {
			s.fail(w, r, err)
			return
		}
	}
	now := time.Now().UTC()
	lb.Status, lb.Round, lb.RoundStart, lb.Invited = "live", 1, now, nil
	lb.Last = make([]game.Lineup, len(lb.Seats))
	if err := s.st.SaveLobby(r.Context(), lb); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": lb.ID})
}
