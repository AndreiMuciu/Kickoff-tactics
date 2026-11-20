package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"footy/internal/game"
	"footy/internal/store"
)

// A series is a live season between two friends. Every round runs on a fixed
// clock that starts at RoundStart:
//
//	transfer window -> countdown -> first half -> break -> second half -> (shootout) -> result
//
// Both managers poll the server. It only ever hands out frames that have
// already "happened", so neither can look ahead and both see the same moment
// at 1x speed. All state is in the database; phase changes are applied lazily
// by whichever poll arrives first, guarded by optimistic versions.
var (
	shopFirst     = 50 * time.Second // first transfer window: time to read the board
	shopLater     = 40 * time.Second
	countdownDur  = 4 * time.Second
	breakDur      = 20 * time.Second
	resultDur     = 9 * time.Second
	lookahead     = 3 * time.Second // frames buffered ahead for smooth playback
	inviteTTL     = 60 * time.Second
	tickDur       = 100 * time.Millisecond
	abandonedFor  = 3 * time.Minute // nobody polled for this long: the game is over
	orderFreezeAt = 300 * time.Millisecond
	halfDur       = time.Duration(game.HalfTicks) * tickDur
)

// SetSpeed makes every phase of live games run `x` times faster. It exists for
// testing whole lobbies quickly (GAME_SPEED); clients follow because the phase
// lengths are sent to them.
func SetSpeed(x float64) {
	if x <= 1 || x > 50 {
		return
	}
	for _, d := range []*time.Duration{&shopFirst, &shopLater, &countdownDur, &breakDur, &resultDur, &lookahead, &tickDur, &halfDur, &orderFreezeAt, &queueWait, &rankedWait} {
		*d = time.Duration(float64(*d) / x)
	}
}

type roundClock struct{ shopEnd, kickoff, halfEnd, h2Start, matchEnd, pensEnd, roundEnd time.Time }

func clockOf(sr *store.Series) roundClock {
	return clockAt(sr.Round, sr.RoundStart, sr.PenTicks, false)
}

// clockAt lays out one round on the shared clock. A penalty round (`kicks`)
// has no halves: the shootout starts at kick-off.
func clockAt(round int, start time.Time, penTicks int, kicks bool) roundClock {
	shop := shopLater
	if round <= 1 {
		shop = shopFirst
	}
	var c roundClock
	c.shopEnd = start.Add(shop)
	c.kickoff = c.shopEnd.Add(countdownDur)
	half, brk := halfDur, breakDur
	if kicks {
		half, brk = 0, 0
	}
	c.halfEnd = c.kickoff.Add(half)
	c.h2Start = c.halfEnd.Add(brk)
	c.matchEnd = c.h2Start.Add(half)
	// A drawn match is followed by a shootout, as long as its recorded frames.
	c.pensEnd = c.matchEnd.Add(time.Duration(penTicks) * tickDur)
	c.roundEnd = c.pensEnd.Add(resultDur)
	return c
}

func seriesRunKey(seriesID, uid string) string { return "vs:" + seriesID + ":" + uid }

func roleOf(sr *store.Series, uid string) int {
	if sr.B == uid {
		return 1
	}
	return 0
}

func (s *Server) seriesRuns(ctx context.Context, sr *store.Series) ([2]*game.Run, error) {
	var out [2]*game.Run
	for i, uid := range [2]string{sr.A, sr.B} {
		r, err := s.st.GetRun(ctx, seriesRunKey(sr.ID, uid))
		if err != nil {
			return out, err
		}
		out[i] = r
	}
	return out, nil
}

// activeSeries returns the user's current invite or series, dropping expired invites.
func (s *Server) activeSeries(r *http.Request, uid string) *store.Series {
	sr, err := s.st.ActiveSeries(r.Context(), uid)
	if err != nil {
		return nil
	}
	if sr.Status == "invited" && time.Since(sr.CreatedAt) > inviteTTL {
		_ = s.st.DeleteSeries(r.Context(), sr.ID)
		return nil
	}
	return sr
}

func (s *Server) finishSeries(ctx context.Context, sr *store.Series, note string) {
	names := [2]string{sr.NameA, sr.NameB}
	wins := [2]int{}
	for _, res := range sr.Results {
		wins[res.Winner]++
	}
	line := "🔴 Live series: " + names[sr.Winner] + " beat " + names[1-sr.Winner] + " " + strconv.Itoa(wins[sr.Winner]) + "–" + strconv.Itoa(wins[1-sr.Winner]) + note
	if id, err := newID(); err == nil {
		_ = s.st.AddMessage(ctx, &store.Message{ID: id, Pair: store.PairID(sr.A, sr.B), Text: line, System: true, At: time.Now().UTC()})
	}
	for i, uid := range [2]string{sr.A, sr.B} {
		d := map[string]int{"series": 1}
		if sr.Winner == i {
			d["seriesWins"] = 1
		}
		if run, err := s.st.GetRun(ctx, seriesRunKey(sr.ID, uid)); err == nil {
			addMatches(d, run)
		}
		_ = s.st.AddStats(ctx, uid, d)
	}
}

// advance moves the series through any phase boundaries that have passed and
// saves it once. On a version conflict another poll already did the work, so
// the caller just reloads.
func (s *Server) advance(ctx context.Context, sr *store.Series, now time.Time) error {
	if sr.Status != "live" {
		return nil
	}
	changed, finished := false, ""
	for step := 0; step < 6; step++ {
		c := clockOf(sr)
		switch {
		case now.Sub(c.roundEnd) > abandonedFor:
			// Both managers walked away: close it for whoever is ahead.
			runs, err := s.seriesRuns(ctx, sr)
			if err != nil {
				return err
			}
			sr.Winner = 0
			if runs[1].Patience > runs[0].Patience {
				sr.Winner = 1
			}
			sr.Status, finished, changed = "done", " (abandoned)", true
		case !sr.Frozen && !now.Before(c.shopEnd):
			runs, err := s.seriesRuns(ctx, sr)
			if err != nil {
				return err
			}
			names := [2]string{sr.NameA, sr.NameB}
			for i, run := range runs {
				if run.Deadline() {
					_ = s.st.SaveRun(ctx, run)
				}
				sr.Squads[i], _ = run.Squad(names[i])
			}
			seed, commit, err := game.NewSeed()
			if err != nil {
				return err
			}
			sr.Seed, sr.Commit = seed, commit
			sr.Orders = [2]string{"balanced", "balanced"}
			sr.Frozen, changed = true, true
			continue
		case sr.Frozen && !sr.Settled && !now.Before(c.matchEnd):
			res, err := game.SimulateSeed(sr.Seed, sr.Squads, sr.Orders, 2)
			if err != nil {
				return err
			}
			runs, err := s.seriesRuns(ctx, sr)
			if err != nil {
				return err
			}
			names := [2]string{sr.NameA, sr.NameB}
			for i, run := range runs {
				// Idempotent per round, so a retry after a conflict cannot settle twice.
				if run.SettleVersus(sr.Round, names[1-i], res.Score[i], res.Score[1-i], res.Winner == i, len(res.Pens) > 0, res.SentOff, i, sr.Round) {
					if err := s.st.SaveRun(ctx, run); err != nil {
						return err
					}
				}
			}
			sr.Results = append(sr.Results, store.SeriesResult{Round: sr.Round, Score: res.Score, Winner: res.Winner, Pens: len(res.Pens) > 0})
			sr.LastSquads = sr.Squads
			sr.PenTicks = max(0, len(res.Frames)-2*game.HalfTicks)
			sr.Settled, changed = true, true
			// Decide the series: someone is out of patience, or the round cap is reached.
			p := [2]int{runs[0].Patience, runs[1].Patience}
			if p[0] <= 0 || p[1] <= 0 || sr.Round >= game.MaxVersusRounds {
				switch {
				case p[0] > p[1]:
					sr.Winner = 0
				case p[1] > p[0]:
					sr.Winner = 1
				default:
					sr.Winner = res.Winner
				}
			}
			continue
		case sr.Settled && !now.Before(c.roundEnd):
			if sr.Winner >= 0 {
				sr.Status, finished, changed = "done", "", true
				break
			}
			sr.Round++
			sr.RoundStart = c.roundEnd
			if now.Sub(c.roundEnd) > 5*time.Second {
				sr.RoundStart = now // nobody was watching: restart the clock from now
			}
			sr.Frozen, sr.Settled, sr.PenTicks, changed = false, false, 0, true
			continue
		}
		break
	}
	if !changed {
		return nil
	}
	if err := s.st.SaveSeries(ctx, sr); err != nil {
		return err
	}
	if sr.Status == "done" {
		s.finishSeries(ctx, sr, finished)
	}
	return nil
}

// loadSeries returns the series by id for one of its two managers, advanced to now.
func (s *Server) loadSeries(r *http.Request, id, uid string) (*store.Series, bool) {
	for try := 0; try < 4; try++ {
		sr, err := s.st.GetSeries(r.Context(), id)
		if err != nil || (sr.A != uid && sr.B != uid) {
			return nil, false
		}
		err = s.advance(r.Context(), sr, time.Now())
		if err == nil {
			return sr, true
		}
		if !errors.Is(err, store.ErrConflict) {
			s.log.Error("series advance failed", "err", err)
			return nil, false
		}
	}
	return nil, false
}

func (s *Server) seriesInvite(w http.ResponseWriter, r *http.Request, uid string) {
	var b nameBody
	if !decode(w, r, &b) {
		return
	}
	friend, ok := s.friendOf(w, r, uid, b.Username)
	if !ok {
		return
	}
	if !s.duel.allow(uid) {
		writeErr(w, http.StatusTooManyRequests, "Give it a moment before the next invite.")
		return
	}
	me, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if s.activeSeries(r, uid) != nil {
		writeErr(w, http.StatusConflict, "You already have a live series or invite.")
		return
	}
	if s.activeSeries(r, friend.ID) != nil {
		writeErr(w, http.StatusConflict, friend.Name+" is busy with another live series.")
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sr := &store.Series{ID: id, A: uid, B: friend.ID, NameA: me.Name, NameB: friend.Name, Status: "invited", Winner: -1, CreatedAt: time.Now().UTC()}
	if err := s.st.SaveSeries(r.Context(), sr); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": sr.ID})
}

func (s *Server) seriesRespond(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Accept bool `json:"accept"`
	}
	if !decode(w, r, &b) {
		return
	}
	sr := s.activeSeries(r, uid)
	// Only the invited manager answers, and only while the invite is open.
	if sr == nil || sr.Status != "invited" || sr.B != uid {
		writeErr(w, http.StatusNotFound, "No live invite to answer.")
		return
	}
	if !b.Accept {
		_ = s.st.DeleteSeries(r.Context(), sr.ID)
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	// Both managers start from scratch: a fresh club each, independent of their solo seasons.
	for _, u := range [2]string{sr.A, sr.B} {
		run, err := game.NewVersusRun(seriesRunKey(sr.ID, u))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.st.SaveRun(r.Context(), run); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	sr.Status, sr.Round, sr.RoundStart = "live", 1, time.Now().UTC()
	if err := s.st.SaveSeries(r.Context(), sr); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": sr.ID})
}

func (s *Server) seriesCancel(w http.ResponseWriter, r *http.Request, uid string) {
	sr := s.activeSeries(r, uid)
	if sr == nil || sr.Status != "invited" || sr.A != uid {
		writeErr(w, http.StatusNotFound, "No pending invite to cancel.")
		return
	}
	_ = s.st.DeleteSeries(r.Context(), sr.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// seriesLeave forfeits a running series to the other manager.
func (s *Server) seriesLeave(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &b) {
		return
	}
	for try := 0; try < 4; try++ {
		sr, ok := s.loadSeries(r, b.ID, uid)
		if !ok || sr.Status != "live" {
			writeErr(w, http.StatusNotFound, "No live series.")
			return
		}
		sr.Winner, sr.Status = 1-roleOf(sr, uid), "done"
		err := s.st.SaveSeries(r.Context(), sr)
		if err == nil {
			s.finishSeries(r.Context(), sr, " (forfeit)")
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

// seriesAction is a transfer-window move on the caller's own series club.
func (s *Server) seriesAction(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		ID string `json:"id"`
		game.Action
	}
	if !decode(w, r, &b) {
		return
	}
	sr, ok := s.loadSeries(r, b.ID, uid)
	if !ok || sr.Status != "live" {
		writeErr(w, http.StatusNotFound, "No live series.")
		return
	}
	// The market stays open during the match: the squad playing it is frozen
	// in the series, so a signing counts from the next match on. Only the
	// instant the window closes is off limits, while the squads are frozen.
	if !sr.Frozen && !time.Now().Before(clockOf(sr).shopEnd) {
		writeErr(w, http.StatusConflict, "The squads are being sent in. Try again in a second.")
		return
	}
	run, err := s.st.GetRun(r.Context(), seriesRunKey(sr.ID, uid))
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
	writeJSON(w, 200, s.boardView(sr, run, roleOf(sr, uid)))
}

func (s *Server) seriesOrder(w http.ResponseWriter, r *http.Request, uid string) {
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
	// Both managers may submit at the same instant: retry on a version conflict.
	for try := 0; try < 4; try++ {
		sr, ok := s.loadSeries(r, b.ID, uid)
		if !ok || sr.Status != "live" || !sr.Frozen {
			writeErr(w, http.StatusNotFound, "No match in progress.")
			return
		}
		c, now := clockOf(sr), time.Now()
		// Orders are accepted only during the break, and freeze just before it ends.
		if now.Before(c.halfEnd) || !now.Before(c.h2Start.Add(-orderFreezeAt)) {
			writeErr(w, http.StatusConflict, "Team talks are only possible during half-time.")
			return
		}
		sr.Orders[roleOf(sr, uid)] = b.Tactic
		err := s.st.SaveSeries(r.Context(), sr)
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

// boardView is the caller's club plus what he is allowed to know about the
// opponent: the squad that opponent fielded in the previous match.
func (s *Server) boardView(sr *store.Series, run *game.Run, role int) game.RunView {
	v := run.View()
	names := [2]string{sr.NameA, sr.NameB}
	scout := sr.LastSquads[1-role]
	if sr.Frozen {
		scout = sr.Squads[1-role]
	}
	v.Opponent = game.Opponent{Name: names[1-role], Units: game.LineupUnits(scout)}
	v.OppSynergies = scout.Synergies()
	v.Round = sr.Round
	v.MaxRounds = game.MaxVersusRounds
	v.MaxPatience = game.VersusPatience
	v.LossCost = versusLossCost(run, sr.Round)
	return v
}

// versusLossCost is what a defeat in this live round costs the club, by one
// goal and by three or more. A penalty round (stake 0) costs nothing.
func versusLossCost(run *game.Run, stake int) [2]int {
	if stake <= 0 {
		return [2]int{}
	}
	return [2]int{run.LossFor(game.VersusDamage(stake, 1)), run.LossFor(game.VersusDamage(stake, 3))}
}

type seriesView struct {
	Status   string    `json:"status"` // none | invited | live | done
	ID       string    `json:"id,omitempty"`
	Role     int       `json:"role"`
	Opponent string    `json:"opponent"`
	Names    [2]string `json:"names"`

	Round       int    `json:"round"`
	MaxRounds   int    `json:"maxRounds"`
	Phase       string `json:"phase,omitempty"` // shop | countdown | h1 | halftime | h2 | result | done
	ServerNow   int64  `json:"serverNow"`
	PhaseEndsAt int64  `json:"phaseEndsAt"`
	KickoffAt   int64  `json:"kickoffAt"`
	HalfMs      int64  `json:"halfMs"`
	BreakMs     int64  `json:"breakMs"`
	PenTicks    int    `json:"penTicks"` // shootout length, known once the match has ended
	PenRound    bool   `json:"penRound"` // this round is a penalty shootout and nothing else

	My          *game.RunView `json:"my,omitempty"`
	Patience    [2]int        `json:"patience"`
	MaxPatience int           `json:"maxPatience"`
	Wins        [2]int        `json:"wins"`

	Commit string             `json:"commit,omitempty"`
	Roster []game.RosterEntry `json:"roster,omitempty"`
	From   int                `json:"from"`
	Frames []game.Frame       `json:"frames"`
	Events []game.Event       `json:"events"`

	HalfScore *[2]int              `json:"halfScore,omitempty"`
	Stats     *[2]game.TeamStats   `json:"stats,omitempty"`
	MyOrder   string               `json:"myOrder,omitempty"`
	Score     *[2]int              `json:"score,omitempty"`
	Pens      []game.PenKick       `json:"pens,omitempty"`
	MatchWon  *int                 `json:"matchWinner,omitempty"`
	Seed      string               `json:"seed,omitempty"`
	Results   []store.SeriesResult `json:"results"`
	Winner    *int                 `json:"winner,omitempty"` // series winner, once decided
}

func (s *Server) seriesGet(w http.ResponseWriter, r *http.Request, uid string) {
	q := r.URL.Query()
	empty := seriesView{Status: "none", Frames: []game.Frame{}, Events: []game.Event{}, Results: []store.SeriesResult{}}
	var sr *store.Series
	if id := q.Get("id"); id != "" {
		// A known series is followed by id, so its final state stays readable.
		got, ok := s.loadSeries(r, id, uid)
		if !ok {
			writeErr(w, http.StatusNotFound, "No such live series.")
			return
		}
		sr = got
	} else {
		active := s.activeSeries(r, uid)
		if active == nil {
			writeJSON(w, 200, empty)
			return
		}
		if active.Status == "live" {
			got, ok := s.loadSeries(r, active.ID, uid)
			if !ok || got.Status != "live" {
				writeJSON(w, 200, empty)
				return
			}
			active = got
		}
		sr = active
	}
	now := time.Now()
	role := roleOf(sr, uid)
	names := [2]string{sr.NameA, sr.NameB}
	v := empty
	v.Status, v.ID, v.Role, v.Names, v.Opponent = sr.Status, sr.ID, role, names, names[1-role]
	v.ServerNow, v.HalfMs, v.BreakMs = now.UnixMilli(), halfDur.Milliseconds(), breakDur.Milliseconds()
	v.Round, v.MaxRounds, v.MaxPatience = sr.Round, game.MaxVersusRounds, game.VersusPatience
	if sr.Results != nil {
		v.Results = sr.Results
	}
	for _, res := range sr.Results {
		v.Wins[res.Winner]++
	}
	if sr.Status == "invited" {
		writeJSON(w, 200, v)
		return
	}
	runs, err := s.seriesRuns(r.Context(), sr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Patience = [2]int{runs[0].Patience, runs[1].Patience}
	c := clockOf(sr)
	// The round is settled the moment the match ends, but while the shootout is
	// still being watched the table must not give the winner away: show the
	// standings as they were before this round.
	watchingPens := sr.Status == "live" && sr.Settled && sr.PenTicks > 0 && now.Before(c.pensEnd) && len(sr.Results) > 0
	if watchingPens {
		v.Results = sr.Results[:len(sr.Results)-1]
		v.Wins = [2]int{}
		for _, res := range v.Results {
			v.Wins[res.Winner]++
		}
		for i, run := range runs {
			if n := len(run.History); n > 0 && run.History[n-1].Round == sr.Round {
				v.Patience[i] += run.History[n-1].Damage
			}
		}
	}
	board := s.boardView(sr, runs[role], role)
	v.My = &board
	if sr.Winner >= 0 && !watchingPens {
		win := sr.Winner
		v.Winner = &win
	}

	v.KickoffAt = c.kickoff.UnixMilli()
	released, halves := 0, 0
	switch {
	case sr.Status == "done":
		v.Phase, halves, released = "done", 2, 2*game.HalfTicks+sr.PenTicks
		v.PhaseEndsAt = v.ServerNow
	case !sr.Frozen:
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
		released = 2*game.HalfTicks + min(sr.PenTicks, int((now.Sub(c.matchEnd)+lookahead)/tickDur))
	default:
		v.Phase, v.PhaseEndsAt, halves, released = "result", c.roundEnd.UnixMilli(), 2, 2*game.HalfTicks+sr.PenTicks
	}
	v.PenTicks = sr.PenTicks
	// A finished series may have ended by forfeit before any match was frozen.
	if halves == 0 || sr.Seed == "" {
		writeJSON(w, 200, v)
		return
	}
	res, err := game.SimulateSeed(sr.Seed, sr.Squads, sr.Orders, halves)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v.Commit, v.Roster = sr.Commit, res.Roster
	from, _ := strconv.Atoi(q.Get("from"))
	released = min(released, len(res.Frames))
	from = max(0, min(from, released))
	v.From = from
	v.Frames = res.Frames[from:released]
	for _, ev := range res.Events {
		if (ev.T > from || (from == 0 && ev.T == 0)) && ev.T <= released {
			v.Events = append(v.Events, ev)
		}
	}
	if !now.Before(c.halfEnd) || sr.Status == "done" {
		hs := res.HalfScore
		v.HalfScore = &hs
		v.MyOrder = sr.Orders[role]
		if v.Phase == "halftime" {
			st := res.Stats
			v.Stats = &st
		}
	}
	if v.Phase == "shootout" {
		sc := res.Score
		v.Score = &sc // level after 90 minutes; the kicks themselves arrive as frames
	}
	if v.Phase == "result" || v.Phase == "done" {
		sc, st, win := res.Score, res.Stats, res.Winner
		v.Score, v.Stats, v.Pens, v.MatchWon, v.Seed = &sc, &st, res.Pens, &win, sr.Seed
	}
	writeJSON(w, 200, v)
}
