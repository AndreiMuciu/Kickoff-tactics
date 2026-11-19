package api

import (
	"net/http"

	"footy/internal/game"
)

// The team plan is a manager's shopping list: the players he wants to end up
// with. It belongs to the account, so it is the same in a solo season, a live
// series and every lobby. The board marks planned players in the market and
// tells him what is still missing; the plan never buys anything by itself.

type planView struct {
	Players     []string           `json:"players"`
	Synergies   []game.SynergyView `json:"synergies"` // what the planned team would have, all fielded
	MaxOutfield int                `json:"maxOutfield"`
}

// planOf validates a list of player ids: known players, no repeats, at most
// one keeper and as many outfield players as the biggest stadium fields.
func planOf(ids []string) (game.Lineup, string) {
	lu := game.Lineup{GK: game.LineupUnit{DefID: game.AcademyKeeper, Stars: 1}}
	seen := map[string]bool{}
	keepers := 0
	for _, id := range ids {
		d := game.PlayerByID[id]
		switch {
		case d == nil:
			return lu, "Unknown player."
		case seen[id]:
			return lu, "A player can be in the plan only once."
		}
		seen[id] = true
		if game.IsGK(id) {
			if keepers++; keepers > 1 {
				return lu, "The plan has room for one goalkeeper."
			}
			lu.GK = game.LineupUnit{DefID: id, Stars: 1}
			continue
		}
		if len(lu.Units) >= game.MaxLevel {
			return lu, "The plan is full: the biggest stadium fields that many outfield players."
		}
		n := len(lu.Units)
		lu.Units = append(lu.Units, game.LineupUnit{DefID: id, Stars: 1, Line: n / 5, Lane: n % 5})
	}
	return lu, ""
}

func writePlan(w http.ResponseWriter, ids []string) {
	lu, msg := planOf(ids)
	if msg != "" { // a stored plan can only fail this after the player list changed
		ids, lu = []string{}, game.Lineup{GK: game.LineupUnit{DefID: game.AcademyKeeper, Stars: 1}}
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, 200, planView{Players: ids, Synergies: lu.Synergies(), MaxOutfield: game.MaxLevel})
}

func (s *Server) getPlan(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writePlan(w, u.Plan)
}

func (s *Server) setPlan(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Players []string `json:"players"`
	}
	if !decode(w, r, &b) {
		return
	}
	if len(b.Players) > game.MaxLevel+1 {
		writeErr(w, http.StatusUnprocessableEntity, "The plan is full.")
		return
	}
	if _, msg := planOf(b.Players); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.st.SetPlan(r.Context(), uid, b.Players); err != nil {
		s.fail(w, r, err)
		return
	}
	writePlan(w, b.Players)
}
