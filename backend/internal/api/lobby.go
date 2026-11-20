package api

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"footy/internal/game"
	"footy/internal/store"
)

// Ranked lobbies: up to eight managers, bots in the empty seats. Every round
// the survivors are paired off and play each other on the same shared clock as
// a live series; losing costs patience and a manager at zero is out. The
// finishing position moves the manager's ranked points.
const (
	lobbySize      = 8
	queueStale     = 15 * time.Second // a queued manager who stopped polling is dropped
	lobbyWarmup    = 2                // penalty rounds, with no patience at stake, before the matches start
	lobbyMaxRounds = 26 + lobbyWarmup
	baseRating     = 0   // everyone starts from zero points
	rankingLimit   = 100 // rows returned by a ranking, general or friends
)

// queueModes are the two ways to play a lobby: for points, or just for fun.
var queueModes = []string{"ranked", "normal"}

// queueWait is how long a normal queue keeps looking for people before bots
// take the empty seats. rankedWait is the same for ranked: a group of eight
// real managers of similar level starts at once, but nobody waits longer than
// this; then whoever is queued plays, with bots in the empty seats.
var (
	queueWait  = 60 * time.Second
	rankedWait = 2 * time.Minute
)

// A tier is a division of the ranking. Each has its own points table for
// finishing 1st..8th: the lower the division, the more a good finish pays and
// the less a bad one costs, so a manager who is better than his division
// climbs out of it quickly and nobody gets stuck at the bottom.
type tier struct {
	Name   string `json:"name"`
	Min    int    `json:"min"`
	Max    int    `json:"max"`    // -1: no upper limit
	Points [8]int `json:"points"` // change for places 1 to 8
}

var tiers = []tier{
	{"Bronze", 0, 99, [8]int{50, 38, 26, 10, 4, -2, -6, -10}},
	{"Silver", 100, 249, [8]int{46, 34, 22, 10, -2, -8, -14, -20}},
	{"Gold", 250, 449, [8]int{42, 30, 18, 10, -6, -14, -22, -30}},
	{"Platinum", 450, 699, [8]int{38, 26, 14, 10, -10, -20, -30, -40}},
	{"Diamond", 700, -1, [8]int{34, 22, 12, 10, -12, -24, -36, -48}},
}

func tierFor(rating int) tier {
	t := tiers[0]
	for _, x := range tiers {
		if rating >= x.Min {
			t = x
		}
	}
	return t
}

func tierOf(rating int) string { return tierFor(rating).Name }

// Matchmaking by level. A manager first accepts only opponents close to his
// own points; the longer he waits, the wider he looks, and after a while anyone.
const (
	spreadStart   = 100              // points either side at first
	spreadStep    = 50               // widened by this much...
	spreadEvery   = 10 * time.Second // ...this often
	spreadAnyone  = 2 * time.Minute  // after this long, any opponent will do
	spreadNoLimit = 1 << 30
)

func spreadOf(e store.QueueEntry, now time.Time) int {
	waited := now.Sub(e.JoinedAt)
	if waited >= spreadAnyone {
		return spreadNoLimit
	}
	return spreadStart + spreadStep*int(waited/spreadEvery)
}

// pickRanked chooses eight queued managers of similar level, or nil when no
// such group exists yet. Entries are considered in order of points; a group is
// acceptable when its spread fits what every one of its members accepts now.
func pickRanked(entries []store.QueueEntry, now time.Time) []int {
	if len(entries) < lobbySize {
		return nil
	}
	idx := make([]int, len(entries))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return entries[idx[a]].Rating < entries[idx[b]].Rating })
	best, bestGap := -1, 0
	for i := 0; i+lobbySize <= len(idx); i++ {
		gap := entries[idx[i+lobbySize-1]].Rating - entries[idx[i]].Rating
		ok := true
		for _, k := range idx[i : i+lobbySize] {
			if gap > spreadOf(entries[k], now) {
				ok = false
				break
			}
		}
		if ok && (best < 0 || gap < bestGap) {
			best, bestGap = i, gap
		}
	}
	if best < 0 {
		return nil
	}
	return idx[best : best+lobbySize]
}

// fillRanked is the fallback once the longest-waiting ranked manager has waited
// rankedWait: he plays with the queued managers closest to his points, up to
// eight, and bots take the remaining seats. nil while nobody has waited that long.
func fillRanked(entries []store.QueueEntry, now time.Time) []int {
	first := -1
	for i, e := range entries {
		if first < 0 || e.JoinedAt.Before(entries[first].JoinedAt) {
			first = i
		}
	}
	if first < 0 || now.Sub(entries[first].JoinedAt) < rankedWait {
		return nil
	}
	others := make([]int, 0, len(entries)-1)
	for i := range entries {
		if i != first {
			others = append(others, i)
		}
	}
	gap := func(i int) int {
		d := entries[i].Rating - entries[first].Rating
		return max(d, -d)
	}
	sort.SliceStable(others, func(a, b int) bool { return gap(others[a]) < gap(others[b]) })
	return append([]int{first}, others[:min(len(others), lobbySize-1)]...)
}

var botNames = []string{"Bot Mourinho", "Bot Guardiola", "Bot Ancelotti", "Bot Klopp", "Bot Lucescu", "Bot Simeone", "Bot Bielsa", "Bot Iordănescu"}

func lobbyRunKey(lobbyID string, seat int) string {
	return "lb:" + lobbyID + ":" + strconv.Itoa(seat)
}

func seatOf(lb *store.Lobby, uid string) int {
	for i, st := range lb.Seats {
		if st.UserID == uid {
			return i
		}
	}
	return -1
}

func aliveSeats(lb *store.Lobby) []int {
	var out []int
	for i, st := range lb.Seats {
		if st.Alive {
			out = append(out, i)
		}
	}
	return out
}

// pairOf returns the pair a seat plays in this round as itself, and its side.
// A seat used only as somebody's ghost is not "in" that pair.
func pairOf(lb *store.Lobby, seat int) (*store.LobbyPair, int) {
	for i := range lb.Pairs {
		p := &lb.Pairs[i]
		if p.A == seat {
			return p, 0
		}
		if p.B == seat && !p.Ghost {
			return p, 1
		}
	}
	return nil, 0
}

// lobbyClock lays out the lobby's current round; the first rounds are penalty rounds.
func lobbyClock(lb *store.Lobby) roundClock {
	return clockAt(lb.Round, lb.RoundStart, lb.PenTicks, lb.Round <= lobbyWarmup)
}

// playPair simulates one meeting of the round up to `halves` halves; a
// penalty round is the shootout alone, whatever `halves` says.
func playPair(lb *store.Lobby, p *store.LobbyPair, halves int) (*game.SimResult, error) {
	if p.Shootout {
		return game.SimulateShootout(p.Seed, p.Squads)
	}
	return game.SimulateSeed(p.Seed, p.Squads, effectiveOrders(lb, p), halves)
}

// effectiveOrders fills in the half-time order of bots and ghosts, which is a
// pure function of the seed and the half-time score.
func effectiveOrders(lb *store.Lobby, p *store.LobbyPair) [2]string {
	o := p.Orders
	if lb.Seats[p.A].Bot {
		o[0] = game.BotOrder(p.Seed, p.Squads, 0)
	}
	if lb.Seats[p.B].Bot || p.Ghost {
		o[1] = game.BotOrder(p.Seed, p.Squads, 1)
	}
	return o
}

func rngFromHex(seedHex string) *game.RNG {
	b, _ := hex.DecodeString(seedHex)
	var v uint64 = 1
	if len(b) >= 8 {
		v = binary.BigEndian.Uint64(b[:8])
	}
	return game.NewRNG(v)
}

// eliminate marks a seat as out and gives it the next free finishing position.
func eliminate(lb *store.Lobby, seat int) {
	st := &lb.Seats[seat]
	if !st.Alive {
		return
	}
	st.Alive = false
	st.Place = len(aliveSeats(lb)) + 1
	if lb.Ranked {
		// The table of the division the manager started this lobby in.
		st.LPChange = tierFor(st.Rating).Points[st.Place-1]
	}
}

// advanceLobby moves the lobby through any phase boundaries that have passed
// and saves it once. Ranked points are written only after that save succeeds,
// so a lost race cannot rate anybody twice.
func (s *Server) advanceLobby(ctx context.Context, lb *store.Lobby, now time.Time) error {
	if lb.Status != "live" {
		return nil
	}
	changed := false
	for step := 0; step < 6; step++ {
		c := lobbyClock(lb)
		alive := aliveSeats(lb)
		// Between rounds, one manager left (the others forfeited) means it is over.
		if !lb.Frozen && finishIfDecided(lb) {
			changed = true
			break
		}
		switch {
		case now.Sub(c.roundEnd) > abandonedFor:
			// Everybody walked away: close the lobby in the current order.
			sort.SliceStable(alive, func(a, b int) bool { return lb.Seats[alive[a]].Patience < lb.Seats[alive[b]].Patience })
			for _, seat := range alive {
				eliminate(lb, seat)
			}
			lb.Status, changed = "done", true
		case !lb.Frozen && !now.Before(c.shopEnd):
			seed, _, err := game.NewSeed()
			if err != nil {
				return err
			}
			g := rngFromHex(seed)
			squads := map[int]game.Lineup{}
			for _, seat := range alive {
				run, err := s.st.GetRun(ctx, lobbyRunKey(lb.ID, seat))
				if err != nil {
					return err
				}
				dirty := false
				if lb.Seats[seat].Bot {
					run.BotManage(g)
					dirty = true
				}
				if run.Deadline() {
					dirty = true
				}
				if dirty {
					if err := s.st.SaveRun(ctx, run); err != nil {
						return err
					}
				}
				squads[seat], _ = run.Squad(lb.Seats[seat].Name)
			}
			// Random pairings; with an odd number the last one meets a ghost.
			for i := len(alive) - 1; i > 0; i-- {
				j := g.Intn(i + 1)
				alive[i], alive[j] = alive[j], alive[i]
			}
			lb.Pairs = nil
			// The first rounds are penalty rounds between the managers: the
			// keeper and the best shooters on the pitch, five kicks each,
			// a prize for the winner and no patience at stake.
			for i := 0; i < len(alive); i += 2 {
				p := store.LobbyPair{A: alive[i], Orders: [2]string{"balanced", "balanced"}, Shootout: lb.Round <= lobbyWarmup}
				if i+1 < len(alive) {
					p.B = alive[i+1]
				} else {
					p.B, p.Ghost = alive[g.Intn(len(alive)-1)], true
				}
				ps, pc, err := game.NewSeed()
				if err != nil {
					return err
				}
				p.Seed, p.Commit = ps, pc
				p.Squads = [2]game.Lineup{squads[p.A], squads[p.B]}
				lb.Pairs = append(lb.Pairs, p)
			}
			lb.Frozen, changed = true, true
			continue
		case lb.Frozen && !lb.Settled && !now.Before(c.matchEnd):
			lb.PenTicks = 0
			for i := range lb.Pairs {
				p := &lb.Pairs[i]
				res, err := playPair(lb, p, 2)
				if err != nil {
					return err
				}
				p.PenTicks = max(0, len(res.Frames)-2*game.HalfTicks)
				if p.Shootout {
					p.PenTicks = len(res.Frames)
				}
				lb.PenTicks = max(lb.PenTicks, p.PenTicks)
				sides := []int{0}
				if !p.Ghost {
					sides = append(sides, 1)
				}
				for _, side := range sides {
					seat := [2]int{p.A, p.B}[side]
					run, err := s.st.GetRun(ctx, lobbyRunKey(lb.ID, seat))
					if err != nil {
						return err
					}
					// Idempotent per round, so a retry after a conflict cannot settle twice.
					opponent := lb.Seats[[2]int{p.B, p.A}[side]].Name
					if p.NPC != "" {
						opponent = p.NPC
					}
					if run.SettleVersus(lb.Round, opponent, res.Score[side], res.Score[1-side], res.Winner == side, len(res.Pens) > 0, res.SentOff, side, max(0, lb.Round-lobbyWarmup)) {
						if err := s.st.SaveRun(ctx, run); err != nil {
							return err
						}
					}
					lb.Seats[seat].Patience, lb.Seats[seat].Streak = run.Patience, run.Streak
				}
				lb.Last[p.A] = p.Squads[0]
				if !p.Ghost {
					lb.Last[p.B] = p.Squads[1]
				}
			}
			// Managers out of patience leave, the one with least patience first.
			out := []int{}
			for _, seat := range aliveSeats(lb) {
				if lb.Seats[seat].Patience <= 0 {
					out = append(out, seat)
				}
			}
			if lb.Round >= lobbyMaxRounds {
				out = aliveSeats(lb)
			}
			sort.SliceStable(out, func(a, b int) bool { return lb.Seats[out[a]].Patience < lb.Seats[out[b]].Patience })
			if len(out) == len(aliveSeats(lb)) && len(out) > 0 {
				out = out[:len(out)-1] // somebody has to win
			}
			for _, seat := range out {
				eliminate(lb, seat)
			}
			if left := aliveSeats(lb); len(left) == 1 {
				eliminate(lb, left[0]) // the winner takes first place
			}
			lb.Settled, changed = true, true
			continue
		case lb.Settled && !now.Before(c.roundEnd):
			if len(aliveSeats(lb)) == 0 {
				lb.Status, changed = "done", true
				break
			}
			lb.Round++
			lb.RoundStart = c.roundEnd
			if now.Sub(c.roundEnd) > 5*time.Second {
				lb.RoundStart = now
			}
			lb.Frozen, lb.Settled, lb.PenTicks, lb.Pairs, changed = false, false, 0, nil, true
			continue
		}
		break
	}
	if !changed {
		return nil
	}
	return s.saveLobby(ctx, lb)
}

// saveLobby stores the lobby and, in a ranked one, writes the points of every
// manager who has just been placed. Who gets rated is decided before the save
// and written only after it succeeds, so a lost race cannot rate anybody twice.
func (s *Server) saveLobby(ctx context.Context, lb *store.Lobby) error {
	var rate []int
	for i := range lb.Seats {
		if st := &lb.Seats[i]; !st.Bot && st.Place > 0 && !st.Rated {
			st.Rated = true
			rate = append(rate, i)
		}
	}
	if err := s.st.SaveLobby(ctx, lb); err != nil {
		return err
	}
	for _, i := range rate {
		st := lb.Seats[i]
		if lb.Ranked {
			_ = s.st.AddRating(ctx, st.UserID, st.LPChange)
		}
		// Profile counters: the lobby itself, and the matches played in it.
		d := map[string]int{"lobbies": 1, "placeSum": st.Place}
		if st.Place == 1 {
			d["lobbyWins"] = 1
		}
		if st.Place <= 4 {
			d["lobbyTop4"] = 1
		}
		if lb.Ranked {
			d["ranked"] = 1
		}
		if run, err := s.st.GetRun(ctx, lobbyRunKey(lb.ID, i)); err == nil {
			addMatches(d, run)
		}
		if err := s.st.AddStats(ctx, st.UserID, d); err != nil {
			s.log.Error("stats update failed", "err", err)
		}
	}
	return nil
}

// addMatches adds the matches of a finished live club to a stats delta.
func addMatches(d map[string]int, run *game.Run) {
	for _, h := range run.History {
		d["matches"]++
		d["gf"] += h.GF
		d["ga"] += h.GA
		if h.Won {
			d["wins"]++
		}
	}
}

// finishIfDecided ends the lobby as soon as at most one manager is left: he
// takes first place and there is nothing more to play.
func finishIfDecided(lb *store.Lobby) bool {
	left := aliveSeats(lb)
	if len(left) > 1 {
		return false
	}
	for _, seat := range left {
		eliminate(lb, seat)
	}
	lb.Status = "done"
	return true
}

func (s *Server) loadLobby(r *http.Request, id, uid string) (*store.Lobby, int, bool) {
	for try := 0; try < 4; try++ {
		lb, err := s.st.GetLobby(r.Context(), id)
		if err != nil {
			return nil, 0, false
		}
		seat := seatOf(lb, uid)
		if seat < 0 {
			return nil, 0, false
		}
		err = s.advanceLobby(r.Context(), lb, time.Now())
		if err == nil {
			return lb, seat, true
		}
		if !errors.Is(err, store.ErrConflict) {
			s.log.Error("lobby advance failed", "err", err)
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// ---- matchmaking ----

// matchmake keeps the queue fresh for the polling manager and starts a lobby
// when the queue is full, or when the first manager has waited long enough, in
// which case bots take the empty seats. It reports the manager's queue state.
func (s *Server) matchmake(ctx context.Context, q *store.Queue, uid string, now time.Time) (queued bool, err error) {
	ranked := q.ID == "ranked"
	fresh := q.Entries[:0:0]
	for _, e := range q.Entries {
		if e.UserID == uid {
			e.SeenAt = now.UTC() // still waiting
			queued = true
		}
		if now.Sub(e.SeenAt) < queueStale {
			fresh = append(fresh, e)
		}
	}
	q.Entries = fresh
	if !queued {
		return false, nil
	}
	var picked, rest []store.QueueEntry
	if ranked {
		// Ranked: eight real managers of similar level; after rankedWait,
		// whoever is there, with bots in the empty seats.
		chosen := pickRanked(q.Entries, now)
		if chosen == nil {
			chosen = fillRanked(q.Entries, now)
		}
		if chosen == nil {
			return true, s.st.SaveQueue(ctx, q)
		}
		in := map[int]bool{}
		for _, k := range chosen {
			in[k] = true
		}
		for i, e := range q.Entries {
			if in[i] {
				picked = append(picked, e)
			} else {
				rest = append(rest, e)
			}
		}
		// Somebody else's group may have formed: only report "matched" to its members.
		mine := false
		for _, e := range picked {
			mine = mine || e.UserID == uid
		}
		queued = !mine
	} else {
		if len(q.Entries) < lobbySize && now.Sub(q.Entries[0].JoinedAt) < queueWait {
			return true, s.st.SaveQueue(ctx, q)
		}
		n := min(lobbySize, len(q.Entries))
		picked, rest = q.Entries[:n], append([]store.QueueEntry{}, q.Entries[n:]...)
		queued = false
	}
	stillQueued := queued
	id, err := newID()
	if err != nil {
		return true, err
	}
	lb := &store.Lobby{ID: id, Status: "live", Ranked: ranked, Round: 1, RoundStart: now.UTC(), CreatedAt: now.UTC(), Last: make([]game.Lineup, lobbySize)}
	for _, e := range picked {
		lb.Users = append(lb.Users, e.UserID)
		lb.Seats = append(lb.Seats, store.LobbySeat{UserID: e.UserID, Name: e.Name, Alive: true, Patience: game.VersusPatience, Rating: e.Rating})
	}
	for i := len(lb.Seats); i < lobbySize; i++ {
		lb.Seats = append(lb.Seats, store.LobbySeat{Name: botNames[i%len(botNames)], Bot: true, Alive: true, Patience: game.VersusPatience})
	}
	// Take the managers out of the queue first: if that loses a race, nothing was created.
	q.Entries = rest
	if err := s.st.SaveQueue(ctx, q); err != nil {
		return true, err
	}
	for i := range lb.Seats {
		run, err := game.NewVersusRun(lobbyRunKey(lb.ID, i))
		if err != nil {
			return false, err
		}
		if err := s.st.SaveRun(ctx, run); err != nil {
			return false, err
		}
	}
	return stillQueued, s.st.SaveLobby(ctx, lb)
}

// playingLobby returns the live lobby in which the manager is still in the game.
func (s *Server) playingLobby(ctx context.Context, uid string) *store.Lobby {
	lb, err := s.st.ActiveLobby(ctx, uid)
	if err != nil {
		return nil
	}
	if seat := seatOf(lb, uid); lb.Status != "live" || seat < 0 || !lb.Seats[seat].Alive {
		return nil
	}
	return lb
}

type rankedView struct {
	Name    string        `json:"name"`
	Rating  int           `json:"rating"`
	Tier    string        `json:"tier"`
	Games   int           `json:"games"`
	Queued  bool          `json:"queued"`
	Mode    string        `json:"mode"` // the queue the manager waits in: ranked | normal
	InQueue int           `json:"inQueue"`
	WaitMs  int64         `json:"waitMs"`   // how long until bots fill the lobby
	Waited  int64         `json:"waitedMs"` // how long this manager has been searching
	Spread  int           `json:"spread"`   // ranked search: points either side accepted right now, -1 for anyone
	Tiers   []tier        `json:"tiers"`
	Party   *partyView    `json:"party"`   // the friends lobby the manager is gathering in
	Invites []partyInvite `json:"invites"` // friends lobbies the manager is invited to
	LobbyID string        `json:"lobbyId"` // set when the manager sits in a live lobby
	// WatchLobby is a lobby still being played that the manager is already out
	// of: he can go back to watch it to the end. WatchPlace is where he finished.
	WatchLobby string `json:"watchLobby"`
	WatchPlace int    `json:"watchPlace"`
	Top        []struct {
		Name   string `json:"name"`
		Rating int    `json:"rating"`
		Tier   string `json:"tier"`
		Games  int    `json:"games"`
	} `json:"top"`
}

func (s *Server) rankedGet(w http.ResponseWriter, r *http.Request, uid string) {
	now := time.Now()
	me, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := rankedView{Name: me.Name, Rating: max(0, baseRating+me.LP), Tier: tierOf(max(0, baseRating+me.LP)), Games: me.Games}
	if lb := s.playingLobby(r.Context(), uid); lb != nil {
		v.LobbyID = lb.ID
	} else {
		for _, mode := range queueModes {
			for try := 0; try < 3; try++ {
				q, err := s.st.GetQueue(r.Context(), mode)
				if err != nil {
					break
				}
				queued, err := s.matchmake(r.Context(), q, uid, now)
				if errors.Is(err, store.ErrConflict) {
					continue
				}
				if queued {
					v.Queued, v.Mode, v.InQueue = true, mode, len(q.Entries)
					for _, e := range q.Entries {
						if e.UserID == uid {
							v.Waited = now.Sub(e.JoinedAt).Milliseconds()
							if sp := spreadOf(e, now); mode == "ranked" && sp == spreadNoLimit {
								v.Spread = -1
							} else if mode == "ranked" {
								v.Spread = sp
							}
						}
					}
					if len(q.Entries) > 0 {
						wait, oldest := queueWait, q.Entries[0].JoinedAt
						if mode == "ranked" {
							wait = rankedWait
							for _, e := range q.Entries {
								if e.JoinedAt.Before(oldest) {
									oldest = e.JoinedAt
								}
							}
						}
						v.WaitMs = max(0, (wait - now.Sub(oldest)).Milliseconds())
					}
				}
				break
			}
		}
		if lb := s.playingLobby(r.Context(), uid); lb != nil {
			v.LobbyID, v.Queued = lb.ID, false
		} else if lb, err := s.st.ActiveLobby(r.Context(), uid); err == nil && lb.Status == "live" && now.Sub(lobbyClock(lb).roundEnd) < abandonedFor {
			if seat := seatOf(lb, uid); seat >= 0 && !lb.Seats[seat].Alive {
				v.WatchLobby, v.WatchPlace = lb.ID, lb.Seats[seat].Place
			}
		}
	}
	v.Tiers = tiers
	v.Party, v.Invites = s.partyState(r, uid)
	top, _ := s.st.TopRated(r.Context(), 10)
	for _, u := range top {
		v.Top = append(v.Top, struct {
			Name   string `json:"name"`
			Rating int    `json:"rating"`
			Tier   string `json:"tier"`
			Games  int    `json:"games"`
		}{u.Name, max(0, baseRating+u.LP), tierOf(max(0, baseRating+u.LP)), u.Games})
	}
	writeJSON(w, 200, v)
}

func (s *Server) rankedJoin(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Mode string `json:"mode"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Mode != "ranked" && b.Mode != "normal" {
		writeErr(w, http.StatusUnprocessableEntity, "Unknown queue.")
		return
	}
	if s.playingLobby(r.Context(), uid) != nil || s.openParty(r, uid) != nil {
		writeErr(w, http.StatusConflict, "You are already in a lobby.")
		return
	}
	if s.activeSeries(r, uid) != nil {
		writeErr(w, http.StatusConflict, "Finish or forfeit your live series first.")
		return
	}
	me, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// One queue at a time: joining one leaves the other.
	s.leaveQueues(r, uid, b.Mode)
	now := time.Now()
	for try := 0; try < 4; try++ {
		q, err := s.st.GetQueue(r.Context(), b.Mode)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, e := range q.Entries {
			if e.UserID == uid {
				writeJSON(w, 200, map[string]bool{"queued": true})
				return
			}
		}
		q.Entries = append(q.Entries, store.QueueEntry{UserID: uid, Name: me.Name, JoinedAt: now.UTC(), SeenAt: now.UTC(), Rating: max(0, baseRating+me.LP)})
		err = s.st.SaveQueue(r.Context(), q)
		if err == nil {
			writeJSON(w, 201, map[string]bool{"queued": true})
			return
		}
		if !errors.Is(err, store.ErrConflict) {
			s.fail(w, r, err)
			return
		}
	}
	writeErr(w, http.StatusConflict, "Try again.")
}

// leaveQueues takes the manager out of every queue except `keep`.
func (s *Server) leaveQueues(r *http.Request, uid, keep string) {
	for _, mode := range queueModes {
		if mode == keep {
			continue
		}
		for try := 0; try < 4; try++ {
			q, err := s.st.GetQueue(r.Context(), mode)
			if err != nil {
				break
			}
			kept := q.Entries[:0:0]
			for _, e := range q.Entries {
				if e.UserID != uid {
					kept = append(kept, e)
				}
			}
			if len(kept) == len(q.Entries) {
				break
			}
			q.Entries = kept
			if err := s.st.SaveQueue(r.Context(), q); err == nil || !errors.Is(err, store.ErrConflict) {
				break
			}
		}
	}
}

func (s *Server) rankedCancel(w http.ResponseWriter, r *http.Request, uid string) {
	s.leaveQueues(r, uid, "")
	writeJSON(w, 200, map[string]bool{"queued": false})
}

type rankRow struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Points int    `json:"points"`
	Tier   string `json:"tier"`
	Games  int    `json:"games"`
	You    bool   `json:"you"`
}

// ranking lists every manager in the game by points, best first. Managers on
// the same points share a rank.
// ranking lists managers by points. Scope "friends" ranks the caller among
// his friends; otherwise it is everybody. Either way at most rankingLimit rows
// are returned, plus the caller's own row when he is further down.
func (s *Server) ranking(w http.ResponseWriter, r *http.Request, uid string) {
	ctx := r.Context()
	me, err := s.st.UserByID(ctx, uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	friends := r.URL.Query().Get("scope") == "friends"
	var users []store.User
	total := 0
	if friends {
		fs, err := s.st.ListFriendships(ctx, uid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		ids := []string{uid}
		for _, f := range fs {
			if f.Accepted {
				ids = append(ids, other(f, uid))
			}
		}
		byID, err := s.st.UsersByIDs(ctx, ids)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, u := range byID {
			users = append(users, *u)
		}
		sort.Slice(users, func(a, b int) bool {
			return users[a].LP > users[b].LP || (users[a].LP == users[b].LP && users[a].Name < users[b].Name)
		})
		total = len(users)
	} else {
		if users, err = s.st.TopRated(ctx, rankingLimit); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if len(users) > rankingLimit {
		users = users[:rankingLimit]
	}
	rows := make([]rankRow, 0, len(users)+1)
	mine := 0
	for i, u := range users {
		pts := max(0, baseRating+u.LP)
		rank := i + 1
		if i > 0 && rows[i-1].Points == pts {
			rank = rows[i-1].Rank
		}
		rows = append(rows, rankRow{Rank: rank, Name: u.Name, Points: pts, Tier: tierOf(pts), Games: u.Games, You: u.ID == uid})
		if u.ID == uid {
			mine = rank
		}
	}
	if !friends {
		above, all, err := s.st.RankInfo(ctx, me.LP)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		total = all
		if mine == 0 {
			// Below the listed rows: the caller still sees where he stands.
			mine = above + 1
			pts := max(0, baseRating+me.LP)
			rows = append(rows, rankRow{Rank: mine, Name: me.Name, Points: pts, Tier: tierOf(pts), Games: me.Games, You: true})
		}
	}
	writeJSON(w, 200, map[string]any{"players": rows, "total": total, "shown": rankingLimit, "myRank": mine})
}

// ---- playing ----

func (s *Server) lobbyAction(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID string `json:"id"`
		game.Action
	}
	if !decode(w, r, &b) {
		return
	}
	lb, seat, ok := s.loadLobby(r, b.ID, uid)
	if !ok || lb.Status != "live" || !lb.Seats[seat].Alive {
		writeErr(w, http.StatusNotFound, "You are not playing in that lobby.")
		return
	}
	// The market stays open during the match: the squads playing it are frozen
	// in the pairings, so a signing counts from the next round on.
	if !lb.Frozen && !time.Now().Before(lobbyClock(lb).shopEnd) {
		writeErr(w, http.StatusConflict, "The squads are being sent in. Try again in a second.")
		return
	}
	run, err := s.st.GetRun(r.Context(), lobbyRunKey(lb.ID, seat))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := run.Apply(b.Action); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SaveRun(r.Context(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, lobbyBoard(lb, run, seat))
}

func (s *Server) lobbyOrder(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID     string `json:"id"`
		Tactic string `json:"tactic"`
	}
	if !decode(w, r, &b) {
		return
	}
	if _, ok := game.TacticByID[b.Tactic]; !ok {
		writeErr(w, http.StatusUnprocessableEntity, "Unknown team order.")
		return
	}
	for try := 0; try < 4; try++ {
		lb, seat, ok := s.loadLobby(r, b.ID, uid)
		if !ok || lb.Status != "live" || !lb.Frozen {
			writeErr(w, http.StatusNotFound, "No match in progress.")
			return
		}
		p, side := pairOf(lb, seat)
		if p == nil {
			writeErr(w, http.StatusNotFound, "No match in progress.")
			return
		}
		c, now := lobbyClock(lb), time.Now()
		if now.Before(c.halfEnd) || !now.Before(c.h2Start.Add(-orderFreezeAt)) {
			writeErr(w, http.StatusConflict, "Team talks are only possible during half-time.")
			return
		}
		p.Orders[side] = b.Tactic
		err := s.st.SaveLobby(r.Context(), lb)
		if err == nil {
			writeJSON(w, 200, map[string]string{"myOrder": b.Tactic})
			return
		}
		if !errors.Is(err, store.ErrConflict) {
			s.fail(w, r, err)
			return
		}
	}
	writeErr(w, http.StatusConflict, "Try again.")
}

// lobbyLeave gives up: the manager is placed last among those still in.
func (s *Server) lobbyLeave(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &b) {
		return
	}
	for try := 0; try < 4; try++ {
		lb, seat, ok := s.loadLobby(r, b.ID, uid)
		if !ok {
			writeErr(w, http.StatusNotFound, "No such lobby.")
			return
		}
		if lb.Status != "live" || !lb.Seats[seat].Alive {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		eliminate(lb, seat)
		// Mid-round the others still have a match to finish; otherwise check
		// whether the lobby is already decided.
		if len(aliveSeats(lb)) == 0 {
			lb.Status = "done"
		} else if !lb.Frozen || lb.Settled {
			finishIfDecided(lb)
		}
		err := s.saveLobby(r.Context(), lb)
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

func lobbyBoard(lb *store.Lobby, run *game.Run, seat int) game.RunView {
	v := run.View()
	v.Round, v.MaxRounds, v.MaxPatience = lb.Round, lobbyMaxRounds, game.VersusPatience
	v.LossCost = versusLossCost(run, lb.Round-lobbyWarmup)
	// Scouting: this round's opponent once pairings exist, otherwise nobody yet.
	if p, side := pairOf(lb, seat); p != nil {
		opp := [2]int{p.B, p.A}[side]
		v.Opponent = game.Opponent{Name: lb.Seats[opp].Name, Units: game.LineupUnits(p.Squads[1-side])}
		v.OppSynergies = p.Squads[1-side].Synergies()
	} else {
		v.Opponent = game.Opponent{Name: "", Units: []game.Unit{}}
		v.OppSynergies = []game.SynergyView{}
	}
	return v
}

type standing struct {
	Seat     int    `json:"seat"`
	Name     string `json:"name"`
	Bot      bool   `json:"bot"`
	You      bool   `json:"you"`
	Alive    bool   `json:"alive"`
	Place    int    `json:"place"`
	Patience int    `json:"patience"`
	Streak   int    `json:"streak"`   // +n wins or -n defeats in a row
	Facing   bool   `json:"facing"`   // your opponent this round
	Watchers int    `json:"watchers"` // managers spectating this one right now
}

type lobbyView struct {
	seriesView
	Standings    []standing `json:"standings"`
	Ranked       bool       `json:"ranked"`
	Ghost        bool       `json:"ghost"`  // this round you play a copy of someone's squad
	Warmup       bool       `json:"warmup"` // this round you play a small club, with nothing at stake
	WarmupRounds int        `json:"warmupRounds"`
	Alive        bool       `json:"alive"`
	Place        int        `json:"place"`
	LPChange     int        `json:"lpChange"`
	Rating       int        `json:"rating"`
	Tier         string     `json:"tier"`
	// Watching is the seat whose match and board this answer shows when the
	// manager spectates somebody else; -1 when it is his own.
	Watching int          `json:"watching"`
	Watched  *watchedView `json:"watched,omitempty"`
	// Spectators of the match shown, and of your own match.
	Spectators []string `json:"spectators"`
	WatchingMe []string `json:"watchingMe"`
}

// watchedView is what a spectator sees of another club, as in any auto-battler
// where the boards are public: its pitch, combinations, level and budget.
type watchedView struct {
	Seat      int                `json:"seat"`
	Name      string             `json:"name"`
	Level     int                `json:"level"`
	Budget    int                `json:"budget"`
	Patience  int                `json:"patience"`
	Units     []game.Unit        `json:"units"`
	Synergies []game.SynergyView `json:"synergies"`
	Augments  []string           `json:"augments"`
}

func (s *Server) lobbyGet(w http.ResponseWriter, r *http.Request, uid string) {
	q := r.URL.Query()
	lb, seat, ok := s.loadLobby(r, q.Get("id"), uid)
	if !ok {
		writeErr(w, http.StatusNotFound, "No such lobby.")
		return
	}
	now := time.Now()
	me := lb.Seats[seat]
	v := lobbyView{Ranked: lb.Ranked, Alive: me.Alive, Place: me.Place, LPChange: me.LPChange, Watching: -1}
	v.Status, v.ID = lb.Status, lb.ID
	v.Frames, v.Events, v.Results = []game.Frame{}, []game.Event{}, []store.SeriesResult{}
	v.ServerNow, v.HalfMs, v.BreakMs = now.UnixMilli(), halfDur.Milliseconds(), breakDur.Milliseconds()
	v.Round, v.MaxRounds, v.MaxPatience = lb.Round, lobbyMaxRounds, game.VersusPatience
	v.PenRound = lb.Round <= lobbyWarmup
	if u, err := s.st.UserByID(r.Context(), uid); err == nil {
		v.Rating, v.Tier = max(0, baseRating+u.LP), tierOf(max(0, baseRating+u.LP))
	}

	// Spectating: the match (and, during the window, the board) of another seat.
	target := seat
	if ws, err := strconv.Atoi(q.Get("watch")); err == nil && ws >= 0 && ws < len(lb.Seats) && ws != seat {
		target, v.Watching = ws, ws
	}

	// Presence: who is spectating whom, shared by every instance.
	specs := map[int][]string{}
	if lb.Status == "live" {
		w := -1
		if target != seat {
			w = target
		}
		s.cfg.Bus.Spectate(r.Context(), lb.ID, uid, lb.Seats[seat].Name, w)
		specs = s.cfg.Bus.Spectators(r.Context(), lb.ID)
		for _, names := range specs {
			sort.Strings(names)
		}
	}
	v.WatchingMe = append([]string{}, specs[seat]...)
	v.Spectators = []string{}

	c := lobbyClock(lb)
	p, side := pairOf(lb, seat)
	// While a shootout is still being watched, standings stay as they were.
	hide := lb.Status == "live" && lb.Settled && lb.PenTicks > 0 && now.Before(c.pensEnd)
	facing := -1
	if p != nil {
		facing = [2]int{p.B, p.A}[side]
		if p.NPC != "" {
			facing = -1
		}
	}
	v.WarmupRounds = lobbyWarmup
	for i, st := range lb.Seats {
		row := standing{Seat: i, Name: st.Name, Bot: st.Bot, You: i == seat, Alive: st.Alive, Place: st.Place, Patience: st.Patience, Streak: st.Streak, Facing: i == facing, Watchers: len(specs[i])}
		if hide {
			if run, err := s.st.GetRun(r.Context(), lobbyRunKey(lb.ID, i)); err == nil {
				if n := len(run.History); n > 0 && run.History[n-1].Round == lb.Round {
					row.Patience += run.History[n-1].Damage
					row.Streak = game.StreakOf(run.History[:n-1])
					row.Alive, row.Place = true, 0
				}
			}
		}
		v.Standings = append(v.Standings, row)
	}
	if hide {
		v.Alive, v.Place, v.LPChange = true, 0, 0
	}
	sort.SliceStable(v.Standings, func(a, b int) bool {
		x, y := v.Standings[a], v.Standings[b]
		if x.Alive != y.Alive {
			return x.Alive
		}
		if x.Alive {
			return x.Patience > y.Patience
		}
		return x.Place < y.Place
	})
	shown := func(i int) int { // patience as the standings show it
		for _, row := range v.Standings {
			if row.Seat == i {
				return row.Patience
			}
		}
		return 0
	}
	streak := func(i int) int {
		for _, row := range v.Standings {
			if row.Seat == i {
				return row.Streak
			}
		}
		return 0
	}
	if run, err := s.st.GetRun(r.Context(), lobbyRunKey(lb.ID, seat)); err == nil {
		board := lobbyBoard(lb, run, seat)
		v.My = &board
	}
	if target != seat {
		if run, err := s.st.GetRun(r.Context(), lobbyRunKey(lb.ID, target)); err == nil {
			lu, _ := run.Squad(lb.Seats[target].Name)
			v.Watched = &watchedView{Seat: target, Name: lb.Seats[target].Name, Level: run.Level, Budget: run.Budget, Patience: shown(target),
				Units: game.LineupUnits(lu), Synergies: lu.Synergies(), Augments: run.Augments}
			if v.Watched.Augments == nil {
				v.Watched.Augments = []string{}
			}
		}
	}

	// From here on, the match shown is the target's.
	p, side = pairOf(lb, target)
	if p != nil {
		v.Role = side
		v.Ghost = p.Ghost && target == seat
		v.Names = [2]string{lb.Seats[p.A].Name, lb.Seats[p.B].Name}
		v.Opponent = lb.Seats[[2]int{p.B, p.A}[side]].Name
		if p.NPC != "" {
			v.Ghost, v.Warmup = false, true
			v.Names[1], v.Opponent = p.NPC, p.NPC
		}
		v.Patience = [2]int{shown(p.A), shown(p.B)}
		v.Streaks = [2]int{streak(p.A), streak(p.B)}
		if p.Ghost {
			v.Streaks[1] = 0 // a ghost squad has no run of its own
		}
		v.Spectators = append(v.Spectators, specs[p.A]...)
		if p.B != p.A && !p.Ghost {
			v.Spectators = append(v.Spectators, specs[p.B]...)
		}
		sort.Strings(v.Spectators)
		if p.NPC != "" {
			v.Patience[1] = game.VersusPatience // a small club has no owner to upset
			v.Streaks[1] = 0
		}
	}

	v.KickoffAt = c.kickoff.UnixMilli()
	released, halves := 0, 0
	// A penalty round's frames are the kicks alone; a match's shootout follows its two halves.
	pensFrom := 2 * game.HalfTicks
	if v.PenRound {
		pensFrom = 0
	}
	switch {
	case lb.Status == "done":
		v.Phase, v.PhaseEndsAt = "done", v.ServerNow
	case !lb.Frozen:
		v.Phase, v.PhaseEndsAt = "shop", c.shopEnd.UnixMilli()
	case now.Before(c.kickoff):
		v.Phase, v.PhaseEndsAt, halves = "countdown", c.kickoff.UnixMilli(), 1
	case now.Before(c.halfEnd):
		v.Phase, v.PhaseEndsAt, halves = "h1", c.halfEnd.UnixMilli(), 1
		released = min(game.HalfTicks, int((now.Sub(c.kickoff)+lookahead)/tickDur))
	case now.Before(c.h2Start):
		v.Phase, v.PhaseEndsAt, halves, released = "halftime", c.h2Start.UnixMilli(), 1, game.HalfTicks
	case now.Before(c.matchEnd):
		v.Phase, v.PhaseEndsAt, halves = "h2", c.matchEnd.UnixMilli(), 2
		released = min(2*game.HalfTicks, game.HalfTicks+int((now.Sub(c.h2Start)+lookahead)/tickDur))
	case now.Before(c.pensEnd):
		v.Phase, v.PhaseEndsAt, halves = "shootout", c.pensEnd.UnixMilli(), 2
		released = pensFrom + int((now.Sub(c.matchEnd)+lookahead)/tickDur)
	default:
		v.Phase, v.PhaseEndsAt, halves, released = "result", c.roundEnd.UnixMilli(), 2, 1<<30
	}
	// Someone already out, or sitting out this round, has no match to follow.
	if p == nil || halves == 0 {
		if v.Phase != "shop" && v.Phase != "done" && p == nil {
			v.Phase = "waiting"
		}
		writeJSON(w, 200, v)
		return
	}
	res, err := playPair(lb, p, halves)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.PenTicks = p.PenTicks
	v.Commit, v.Roster = p.Commit, res.Roster
	from, _ := strconv.Atoi(q.Get("from"))
	released = min(released, len(res.Frames))
	from = max(0, min(from, released))
	v.From = from
	v.Frames = res.Frames[from:released]
	// Event ticks count from kick-off of a full match; a penalty round's
	// frames start where that match would have ended.
	base := 0
	if p.Shootout {
		base = 2 * game.HalfTicks
	}
	for _, ev := range res.Events {
		t := ev.T - base
		if (t > from || (from == 0 && t == 0)) && t <= released {
			v.Events = append(v.Events, ev)
		}
	}
	if !now.Before(c.halfEnd) && !p.Shootout {
		hs := res.HalfScore
		v.HalfScore = &hs
		if target == seat {
			v.MyOrder = p.Orders[side]
		}
		if v.Phase == "halftime" {
			st := res.Stats
			v.Stats = &st
		}
	}
	if v.Phase == "shootout" {
		sc := res.Score
		v.Score = &sc
	}
	if v.Phase == "result" {
		sc, st, win := res.Score, res.Stats, res.Winner
		v.Score, v.Stats, v.Pens, v.MatchWon, v.Seed = &sc, &st, res.Pens, &win, p.Seed
	}
	writeJSON(w, 200, v)
}
