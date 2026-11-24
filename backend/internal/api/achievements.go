package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"footy/internal/game"
	mailer "footy/internal/mail"
	"footy/internal/store"
)

// An achievement has five levels: Bronze the first time, then Silver, Gold,
// Platinum and Diamond as the same feat is repeated (`Levels` are the counts
// that reach each one). A level is decided on the server only and stored
// with its date (key "<id>" for Bronze, "<id>@<n>" above), so it is never
// lost, not even when ranked points later drop.
type achievement struct {
	ID     string `json:"id"`
	Icon   string `json:"icon"`
	Name   string `json:"name"`
	One    string `json:"-"` // what Bronze asks for when it is a single feat
	Many   string `json:"-"` // the same with %d, for a count
	Stat   string `json:"-"`
	Rating bool   `json:"-"` // measured in ranked points instead of a counter
	Levels [5]int `json:"levels"`
}

var levelNames = [6]string{"", "Bronze", "Silver", "Gold", "Platinum", "Diamond"}

var achievements = []achievement{
	{ID: "first_win", Icon: "⚽", Name: "Winner", One: "Win a match.", Many: "Win %d matches.", Stat: "wins", Levels: [5]int{1, 10, 50, 150, 500}},
	{ID: "matches_100", Icon: "🏟", Name: "Regular", Many: "Play %d matches.", Stat: "matches", Levels: [5]int{10, 50, 100, 250, 1000}},
	{ID: "goals_100", Icon: "🎯", Name: "Goal Machine", Many: "Score %d goals.", Stat: "gf", Levels: [5]int{10, 50, 100, 250, 1000}},
	{ID: "big_win", Icon: "💥", Name: "Thrashing", One: "Win a match by three goals or more.", Many: "Win %d matches by three goals or more.", Stat: "bigWins", Levels: [5]int{1, 5, 15, 40, 100}},
	{ID: "clean_10", Icon: "🧱", Name: "Brick Wall", One: "Keep a clean sheet.", Many: "Keep %d clean sheets.", Stat: "cleanSheets", Levels: [5]int{1, 10, 25, 60, 150}},
	{ID: "streak_5", Icon: "🔥", Name: "On Fire", One: "Win five matches in a row in one game.", Many: "Win five in a row in %d games.", Stat: "streak5", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "three_star", Icon: "⭐", Name: "Legend Built", One: "Finish a game with a three-star player.", Many: "Finish %d games with a three-star player.", Stat: "threeStars", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "full_house", Icon: "🏗", Name: "Full House", One: "Finish a game with the stadium at level 10.", Many: "Finish %d games with the stadium at level 10.", Stat: "fullStadiums", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "season", Icon: "📅", Name: "Survivor", One: "Finish a solo season without being sacked.", Many: "Finish %d solo seasons without being sacked.", Stat: "seasonsSurvived", Levels: [5]int{1, 5, 15, 40, 100}},
	{ID: "champion", Icon: "🏆", Name: "Champions!", One: "Win the solo league.", Many: "Win the solo league %d times.", Stat: "titles", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "unbeaten", Icon: "🛡", Name: "Invincibles", One: "Win the solo league without losing a match.", Many: "Win the solo league unbeaten %d times.", Stat: "unbeatenTitles", Levels: [5]int{1, 2, 5, 10, 25}},
	{ID: "perfect", Icon: "👑", Name: "Perfect Season", One: "Win every match of a solo season.", Many: "Win every match of a solo season %d times.", Stat: "perfectSeasons", Levels: [5]int{1, 2, 5, 10, 25}},
	{ID: "europe_qf", Icon: "✈️", Name: "European Nights", One: "Reach the quarter-finals of a European cup.", Many: "Reach the European quarter-finals %d times.", Stat: "cupQuarters", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "europe_win", Icon: "🌍", Name: "Conquerors of Europe", One: "Win a European cup.", Many: "Win %d European cups.", Stat: "cupWins", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "crown", Icon: "💎", Name: "Kings of the Continent", One: "Win the European Crown.", Many: "Win the European Crown %d times.", Stat: "crownWins", Levels: [5]int{1, 2, 5, 10, 25}},
	{ID: "double", Icon: "🥂", Name: "The Double", One: "Win the league and the European Crown in the same season.", Many: "Win the league and the European Crown in the same season %d times.", Stat: "doubles", Levels: [5]int{1, 2, 5, 10, 25}},
	{ID: "bounce", Icon: "⬆️", Name: "Bounce Back", One: "Win the promotion play-off after relegation.", Many: "Win the promotion play-off %d times.", Stat: "promotions", Levels: [5]int{1, 3, 10, 25, 50}},
	{ID: "lobby", Icon: "🎟", Name: "Lobby Regular", One: "Finish a lobby.", Many: "Finish %d lobbies.", Stat: "lobbies", Levels: [5]int{1, 10, 50, 150, 500}},
	{ID: "top4", Icon: "4️⃣", Name: "Top Four", One: "Finish a lobby in the top 4.", Many: "Finish in the top 4 %d times.", Stat: "lobbyTop4", Levels: [5]int{1, 10, 50, 150, 500}},
	{ID: "lobby_win", Icon: "🥇", Name: "Last One Standing", One: "Win a lobby.", Many: "Win %d lobbies.", Stat: "lobbyWins", Levels: [5]int{1, 5, 20, 50, 150}},
	{ID: "ranked_10", Icon: "🎮", Name: "Ranked Regular", One: "Finish a ranked lobby.", Many: "Finish %d ranked lobbies.", Stat: "ranked", Levels: [5]int{1, 10, 50, 150, 500}},
	{ID: "climber", Icon: "🧗", Name: "Climber", Many: "Reach %d ranked points.", Rating: true, Levels: [5]int{100, 250, 450, 700, 1000}},
	{ID: "series_win", Icon: "🤝", Name: "Bragging Rights", One: "Win a live series against a friend.", Many: "Win %d live series against friends.", Stat: "seriesWins", Levels: [5]int{1, 5, 20, 50, 150}},
	{ID: "feedback", Icon: "💬", Name: "Suggestion Box", One: "Send feedback from the game.", Many: "Send feedback %d times.", Stat: "feedback", Levels: [5]int{1, 3, 10, 25, 50}},
}

// oldKeys are achievements from before levels, now a level of another one:
// their unlock date carries over.
var oldKeys = map[string]string{
	"first_win@3": "wins_50", "champion@2": "champion_5", "top4@2": "top4_10", "lobby_win@2": "lobby_win_5",
	"climber": "silver", "climber@2": "gold", "climber@3": "platinum", "climber@4": "diamond",
}

var achievementByID = func() map[string]achievement {
	m := map[string]achievement{}
	for _, a := range achievements {
		m[a.ID] = a
	}
	return m
}()

// levelKey is where level n (1-5) of an achievement is stored.
func levelKey(id string, n int) string {
	if n == 1 {
		return id
	}
	return id + "@" + strconv.Itoa(n)
}

// levelAt is when the manager reached level n, if he has.
func levelAt(u *store.User, id string, n int) (time.Time, bool) {
	key := levelKey(id, n)
	if at, ok := u.Achieved[key]; ok {
		return at, true
	}
	if old, ok := oldKeys[key]; ok {
		at, ok := u.Achieved[old]
		return at, ok
	}
	return time.Time{}, false
}

// count is the manager's number for an achievement.
func count(a achievement, u *store.User) int {
	if a.Rating {
		return max(0, baseRating+u.LP)
	}
	return u.Stats[a.Stat]
}

func (a achievement) describe(target int) string {
	if target == 1 && a.One != "" {
		return a.One
	}
	return fmt.Sprintf(a.Many, target)
}

// recordStats adds to the manager's counters and unlocks every level they
// now earn. It returns the level keys unlocked by this call.
func (s *Server) recordStats(ctx context.Context, uid string, d map[string]int) []string {
	if err := s.st.AddStats(ctx, uid, d); err != nil {
		s.log.Error("stats update failed", "err", err)
		return nil
	}
	return s.unlock(ctx, uid)
}

// unlock stores the levels the manager has earned but not yet received.
func (s *Server) unlock(ctx context.Context, uid string) []string {
	u, err := s.st.UserByID(ctx, uid)
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	got := map[string]time.Time{}
	var keys []string
	for _, a := range achievements {
		n := count(a, u)
		for lvl := 1; lvl <= 5; lvl++ {
			if n < a.Levels[lvl-1] {
				break
			}
			if _, had := levelAt(u, a.ID, lvl); !had {
				key := levelKey(a.ID, lvl)
				got[key] = now
				keys = append(keys, key)
			}
		}
	}
	if len(got) == 0 {
		return nil
	}
	if err := s.st.SetAchieved(ctx, uid, got); err != nil {
		s.log.Error("achievement update failed", "err", err)
		return nil
	}
	return keys
}

type achievementView struct {
	ID     string  `json:"id"`
	Icon   string  `json:"icon"`
	Name   string  `json:"name"`
	Desc   string  `json:"desc"`   // what the next level asks for (the last one once at Diamond)
	Level  int     `json:"level"`  // 0 locked, 1 Bronze ... 5 Diamond
	Count  int     `json:"count"`  // how many times so far
	Next   int     `json:"next"`   // the count the next level needs; 0 at Diamond
	Levels [5]int  `json:"levels"` // the count of every level
	At     []int64 `json:"at"`     // when each reached level was unlocked (ms)
	New    bool    `json:"new,omitempty"`
}

func achievementViews(u *store.User) []achievementView {
	out := make([]achievementView, 0, len(achievements))
	for _, a := range achievements {
		v := achievementView{ID: a.ID, Icon: a.Icon, Name: a.Name, Levels: a.Levels, Count: count(a, u), At: []int64{}}
		for lvl := 1; lvl <= 5; lvl++ {
			at, ok := levelAt(u, a.ID, lvl)
			if !ok {
				break
			}
			v.Level = lvl
			v.At = append(v.At, at.UnixMilli())
			v.New = v.New || at.After(u.AchSeen)
		}
		if v.Level < 5 {
			v.Next = a.Levels[v.Level]
			v.Desc = a.describe(v.Next)
		} else {
			v.Desc = a.describe(a.Levels[4])
		}
		out = append(out, v)
	}
	return out
}

// unseenAchievements counts levels unlocked since the manager last looked.
func unseenAchievements(u *store.User) int {
	n := 0
	for _, at := range u.Achieved {
		if at.After(u.AchSeen) {
			n++
		}
	}
	return n
}

// unlockedView is one level just reached, for a toast.
type unlockedView struct {
	ID    string `json:"id"`
	Icon  string `json:"icon"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	Tier  string `json:"tier"` // Bronze ... Diamond
}

func views(keys []string) []unlockedView {
	out := []unlockedView{}
	for _, k := range keys {
		id, lvl := k, 1
		if i := strings.LastIndex(k, "@"); i > 0 {
			id = k[:i]
			lvl, _ = strconv.Atoi(k[i+1:])
		}
		a := achievementByID[id]
		out = append(out, unlockedView{ID: id, Icon: a.Icon, Name: a.Name, Level: lvl, Tier: levelNames[lvl]})
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

// cupStats adds how the run after the league ended: Europe or the play-off.
func cupStats(d map[string]int, run *game.Run) {
	c := run.Cup
	if c == nil {
		return
	}
	switch c.Out {
	case "quarter", "semi", "final", "winner":
		d["cupQuarters"]++
	case "promoted":
		d["promotions"]++
	}
	if c.Out == "winner" {
		d["cupWins"]++
		if c.ID == "crown" {
			d["crownWins"]++
			if run.Result == "champion" {
				d["doubles"]++
			}
		}
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
