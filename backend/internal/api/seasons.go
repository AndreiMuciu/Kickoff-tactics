package api

import (
	"context"
	"sync"
	"time"

	"footy/internal/store"
)

// Ranked seasons follow the calendar month: season 1 is November 2026. When a
// season ends every manager keeps half his points and, if he played ranked
// in it, a badge with the division he finished in. The reset runs lazily, the
// first time any server needs the ranking in a new month; it is safe to run
// twice (see store.RollSeason).
var seasonEpoch = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)

func seasonAt(t time.Time) int {
	t = t.UTC()
	n := (t.Year()-seasonEpoch.Year())*12 + int(t.Month()) - int(seasonEpoch.Month()) + 1
	return max(1, n)
}

// seasonEnd is when season n ends: the first day of the following month.
func seasonEnd(n int) time.Time {
	return time.Date(seasonEpoch.Year(), seasonEpoch.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
}

func tierMins() []store.TierMin {
	out := make([]store.TierMin, 0, len(tiers))
	for _, t := range tiers {
		out = append(out, store.TierMin{Name: t.Name, Min: t.Min - baseRating})
	}
	return out
}

// ensureSeason makes sure the season reset for the current month has run and
// returns the current season. After the first success it costs one atomic read.
func (s *Server) ensureSeason(ctx context.Context) int {
	cur := seasonAt(time.Now())
	if s.seasonRolled.Load() >= int64(cur) {
		return cur
	}
	if err := s.st.RollSeason(ctx, cur, tierMins()); err != nil {
		s.log.Error("season reset failed", "season", cur, "err", err)
		return cur
	}
	s.seasonRolled.Store(int64(cur))
	return cur
}

type seasonView struct {
	Number int   `json:"number"`
	EndsAt int64 `json:"endsAt"`
}

func (s *Server) season(ctx context.Context) seasonView {
	n := s.ensureSeason(ctx)
	return seasonView{Number: n, EndsAt: seasonEnd(n).UnixMilli()}
}

// The top ten on Home is read by every signed-in client every two seconds.
// Points change only when a lobby ends, so one read per server every few
// seconds is plenty.
const topTTL = 5 * time.Second

type topCache struct {
	mu    sync.Mutex
	at    time.Time
	users []store.User
}

func (s *Server) topTen(ctx context.Context) []store.User {
	s.top.mu.Lock()
	defer s.top.mu.Unlock()
	if time.Since(s.top.at) < topTTL {
		return s.top.users
	}
	users, err := s.st.TopRated(ctx, 10)
	if err != nil {
		return s.top.users // the last known table beats an empty one
	}
	s.top.at, s.top.users = time.Now(), users
	return users
}
