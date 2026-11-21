package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"footy/internal/game"
	mailer "footy/internal/mail"
	"footy/internal/store"
)

// An achievement is unlocked when one of the manager's counters reaches a
// target, or (Tier) when his ranked points reach a division. Unlocking is
// decided on the server only and stored with its date, so it is never lost
// when points later drop.
type achievement struct {
	ID     string `json:"id"`
	Icon   string `json:"icon"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Stat   string `json:"-"`
	Target int    `json:"target"`
	Tier   string `json:"-"` // the division to reach, instead of a counter
}

var achievements = []achievement{
	{ID: "first_win", Icon: "⚽", Name: "First Win", Desc: "Win a match.", Stat: "wins", Target: 1},
	{ID: "wins_50", Icon: "💪", Name: "Habit of Winning", Desc: "Win 50 matches.", Stat: "wins", Target: 50},
	{ID: "matches_100", Icon: "🏟", Name: "Centurion", Desc: "Play 100 matches.", Stat: "matches", Target: 100},
	{ID: "goals_100", Icon: "🎯", Name: "Goal Machine", Desc: "Score 100 goals.", Stat: "gf", Target: 100},
	{ID: "big_win", Icon: "💥", Name: "Thrashing", Desc: "Win a match by three goals or more.", Stat: "bigWins", Target: 1},
	{ID: "clean_10", Icon: "🧱", Name: "Brick Wall", Desc: "Keep 10 clean sheets.", Stat: "cleanSheets", Target: 10},
	{ID: "streak_5", Icon: "🔥", Name: "On Fire", Desc: "Win five matches in a row in one game.", Stat: "streak5", Target: 1},
	{ID: "three_star", Icon: "⭐", Name: "Legend Built", Desc: "Finish a game with a three-star player.", Stat: "threeStars", Target: 1},
	{ID: "full_house", Icon: "🏗", Name: "Full House", Desc: "Finish a game with the stadium at level 10.", Stat: "fullStadiums", Target: 1},
	{ID: "season", Icon: "📅", Name: "Survivor", Desc: "Finish a solo season without being sacked.", Stat: "seasonsSurvived", Target: 1},
	{ID: "champion", Icon: "🏆", Name: "Champions!", Desc: "Win the solo league.", Stat: "titles", Target: 1},
	{ID: "champion_5", Icon: "🏛", Name: "Dynasty", Desc: "Win the solo league five times.", Stat: "titles", Target: 5},
	{ID: "unbeaten", Icon: "🛡", Name: "Invincibles", Desc: "Win the solo league without losing a match.", Stat: "unbeatenTitles", Target: 1},
	{ID: "perfect", Icon: "👑", Name: "Perfect Season", Desc: "Win every match of a solo season.", Stat: "perfectSeasons", Target: 1},
	{ID: "lobby", Icon: "🎟", Name: "Debut", Desc: "Finish a lobby.", Stat: "lobbies", Target: 1},
	{ID: "top4", Icon: "4️⃣", Name: "Top Four", Desc: "Finish a lobby in the top 4.", Stat: "lobbyTop4", Target: 1},
	{ID: "top4_10", Icon: "📈", Name: "Consistent", Desc: "Finish in the top 4 ten times.", Stat: "lobbyTop4", Target: 10},
	{ID: "lobby_win", Icon: "🥇", Name: "Last One Standing", Desc: "Win a lobby.", Stat: "lobbyWins", Target: 1},
	{ID: "lobby_win_5", Icon: "🎖", Name: "Serial Winner", Desc: "Win five lobbies.", Stat: "lobbyWins", Target: 5},
	{ID: "ranked_10", Icon: "🎮", Name: "Ranked Regular", Desc: "Finish 10 ranked lobbies.", Stat: "ranked", Target: 10},
	{ID: "silver", Icon: "🥈", Name: "Silver", Desc: "Reach the Silver division.", Tier: "Silver", Target: 1},
	{ID: "gold", Icon: "🥇", Name: "Gold", Desc: "Reach the Gold division.", Tier: "Gold", Target: 1},
	{ID: "platinum", Icon: "💠", Name: "Platinum", Desc: "Reach the Platinum division.", Tier: "Platinum", Target: 1},
	{ID: "diamond", Icon: "💎", Name: "Diamond", Desc: "Reach the Diamond division.", Tier: "Diamond", Target: 1},
	{ID: "series_win", Icon: "🤝", Name: "Bragging Rights", Desc: "Win a live series against a friend.", Stat: "seriesWins", Target: 1},
	{ID: "feedback", Icon: "💬", Name: "Suggestion Box", Desc: "Send feedback from the game.", Stat: "feedback", Target: 1},
}

var achievementByID = func() map[string]achievement {
	m := map[string]achievement{}
	for _, a := range achievements {
		m[a.ID] = a
	}
	return m
}()

// progress is how far the manager is towards an achievement, capped at its target.
func progress(a achievement, u *store.User) int {
	if a.Tier != "" {
		rating := max(0, baseRating+u.LP)
		for _, t := range tiers {
			if t.Name == a.Tier && rating >= t.Min {
				return 1
			}
		}
		return 0
	}
	return min(u.Stats[a.Stat], a.Target)
}

// recordStats adds to the manager's counters and unlocks every achievement
// they now earn. It returns the ids unlocked by this call.
func (s *Server) recordStats(ctx context.Context, uid string, d map[string]int) []string {
	if err := s.st.AddStats(ctx, uid, d); err != nil {
		s.log.Error("stats update failed", "err", err)
		return nil
	}
	return s.unlock(ctx, uid)
}

// unlock stores the achievements the manager has earned but not yet received.
func (s *Server) unlock(ctx context.Context, uid string) []string {
	u, err := s.st.UserByID(ctx, uid)
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	got := map[string]time.Time{}
	var ids []string
	for _, a := range achievements {
		if _, had := u.Achieved[a.ID]; !had && progress(a, u) >= a.Target {
			got[a.ID] = now
			ids = append(ids, a.ID)
		}
	}
	if len(got) == 0 {
		return nil
	}
	if err := s.st.SetAchieved(ctx, uid, got); err != nil {
		s.log.Error("achievement update failed", "err", err)
		return nil
	}
	return ids
}

type achievementView struct {
	achievement
	Progress int   `json:"progress"`
	At       int64 `json:"at,omitempty"` // unlock time (ms), 0 while locked
	New      bool  `json:"new,omitempty"`
}

func achievementViews(u *store.User) []achievementView {
	out := make([]achievementView, 0, len(achievements))
	for _, a := range achievements {
		v := achievementView{achievement: a, Progress: progress(a, u)}
		if at, ok := u.Achieved[a.ID]; ok {
			v.At, v.Progress, v.New = at.UnixMilli(), a.Target, at.After(u.AchSeen)
		}
		out = append(out, v)
	}
	return out
}

// unseenAchievements counts achievements unlocked since the manager last looked.
func unseenAchievements(u *store.User) int {
	n := 0
	for _, at := range u.Achieved {
		if at.After(u.AchSeen) {
			n++
		}
	}
	return n
}

func views(ids []string) []achievement {
	out := []achievement{}
	for _, id := range ids {
		out = append(out, achievementByID[id])
	}
	return out
}

func (s *Server) achievementsSeen(w http.ResponseWriter, r *http.Request, uid string) {
	if err := s.st.SetAchSeen(r.Context(), uid, time.Now().UTC()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- match history ----

const historyPage = 10

// gameStats are the counters a finished game adds beyond its matches.
func gameStats(d map[string]int, run *game.Run) {
	best, cur := 0, 0
	for _, h := range run.History {
		if h.Won {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	if best >= 5 {
		d["streak5"]++
	}
	for _, u := range run.Units {
		if u.Stars >= 3 {
			d["threeStars"]++
			break
		}
	}
	if run.Level >= game.MaxLevel {
		d["fullStadiums"]++
	}
}

// matchStats adds one match to a stats delta.
func matchStats(d map[string]int, h game.HistoryEntry) {
	d["matches"]++
	d["gf"] += h.GF
	d["ga"] += h.GA
	if h.Won {
		d["wins"]++
		if h.GF-h.GA >= 3 {
			d["bigWins"]++
		}
	}
	if h.GA == 0 {
		d["cleanSheets"]++
	}
}

// recordGame writes a finished game to the manager's history.
func (s *Server) recordGame(ctx context.Context, g store.GameRecord, run *game.Run) {
	id, err := newID()
	if err != nil {
		return
	}
	g.ID, g.At = id, time.Now().UTC()
	g.Season = seasonAt(g.At)
	g.Matches = []store.MatchLine{}
	if run != nil {
		for _, h := range run.History {
			g.Matches = append(g.Matches, store.MatchLine{Round: h.Round, Opponent: h.Opponent, GF: h.GF, GA: h.GA, Pens: h.Pens, Won: h.Won, Draw: h.Draw})
			switch {
			case h.Won:
				g.Wins++
			case h.Draw:
				g.Draws++
			default:
				g.Losses++
			}
		}
	}
	if err := s.st.AddGame(ctx, &g); err != nil {
		s.log.Error("history update failed", "err", err)
	}
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, uid string) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	page = min(page, 10000)
	games, total, err := s.st.ListGames(r.Context(), uid, (page-1)*historyPage, historyPage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type row struct {
		store.GameRecord
		At int64 `json:"at"`
	}
	rows := make([]row, 0, len(games))
	for _, g := range games {
		rows = append(rows, row{GameRecord: g, At: g.At.UnixMilli()})
	}
	pages := max(1, (total+historyPage-1)/historyPage)
	writeJSON(w, 200, map[string]any{"games": rows, "page": page, "pages": pages, "total": total})
}

// ---- feedback ----

const (
	feedbackMin = 10
	feedbackMax = 1500
)

var feedbackKinds = map[string]string{"idea": "Idea", "bug": "Bug", "other": "Other"}

// feedbackEvery is how often the game asks a manager whether he has anything
// to say. The first question comes a week after sign-up.
const feedbackEvery = 7 * 24 * time.Hour

func askFeedback(u *store.User, now time.Time) bool {
	last := u.FeedbackAsked
	if last.Before(u.CreatedAt) {
		last = u.CreatedAt
	}
	return now.Sub(last) >= feedbackEvery
}

// feedbackLater answers the weekly question ("not now", or the form was opened).
func (s *Server) feedbackLater(w http.ResponseWriter, r *http.Request, uid string) {
	if err := s.st.SetFeedbackAsked(r.Context(), uid, time.Now().UTC()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) sendFeedback(w http.ResponseWriter, r *http.Request, uid string) {
	var b struct {
		Kind   string `json:"kind"`
		Text   string `json:"text"`
		Screen string `json:"screen"`
	}
	if !decodeMax(w, r, &b, 8<<10) {
		return
	}
	b.Text = strings.TrimSpace(b.Text)
	label, ok := feedbackKinds[b.Kind]
	switch n := utf8.RuneCountInString(b.Text); {
	case !ok:
		writeErr(w, http.StatusBadRequest, "Pick what kind of feedback this is.")
		return
	case n < feedbackMin:
		writeErr(w, http.StatusUnprocessableEntity, "Tell us a little more (at least 10 characters).")
		return
	case n > feedbackMax:
		writeErr(w, http.StatusUnprocessableEntity, "That is too long: keep it under 1500 characters.")
		return
	}
	if !utf8.ValidString(b.Text) {
		writeErr(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if len(b.Screen) > 40 {
		b.Screen = b.Screen[:40]
	}
	if !s.feedback.allow(uid) {
		writeErr(w, http.StatusTooManyRequests, "Thanks! You have sent a lot of feedback already; try again in a while.")
		return
	}
	u, err := s.st.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := &store.Feedback{ID: id, UserID: uid, Name: u.Name, Kind: b.Kind, Text: b.Text, Screen: b.Screen, At: time.Now().UTC()}
	if err := s.st.AddFeedback(r.Context(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	if s.cfg.FeedbackTo != "" {
		s.send(mailer.Message{To: s.cfg.FeedbackTo, Subject: "[Kickoff Tactics] " + label + " from " + u.Name,
			Body: label + " from " + u.Name + " (screen: " + f.Screen + ", " + f.At.Format(time.RFC1123) + ")\n\n" + f.Text + "\n"})
	}
	_ = s.st.SetFeedbackAsked(r.Context(), uid, f.At)
	unlocked := s.recordStats(r.Context(), uid, map[string]int{"feedback": 1})
	writeJSON(w, 200, map[string]any{"ok": true, "unlocked": views(unlocked)})
}
