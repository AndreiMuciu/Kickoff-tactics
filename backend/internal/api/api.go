// Package api is the HTTP edge. It authenticates, rate-limits and validates,
// then hands intents to the game package. It never computes game outcomes.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"footy/internal/bus"
	"footy/internal/game"
	"footy/internal/mail"
	"footy/internal/store"
)

const (
	cookieName  = "footy_sid"
	maxBody     = 2 << 10 // intents are tiny; anything bigger is rejected
	reqTimeout  = 5 * time.Second
	tokenLength = 32
)

type Config struct {
	AllowedOrigins []string
	CookieSecure   bool
	Mailer         mail.Mailer
	PublicURL      string // where the web app is reached; used in mailed links
	FeedbackTo     string // address that receives in-game feedback; empty: stored only
	Bus            bus.Bus
	// TrustProxy: the server sits behind a reverse proxy that appends the real
	// client address to X-Forwarded-For. Never set it when clients connect directly.
	TrustProxy bool
}

type Server struct {
	st       store.Store
	cfg      Config
	origins  map[string]bool
	general  *limiter
	authIP   *limiter // sign-up and login attempts per IP
	authAcc  *limiter // login attempts per account name
	mailTo   *limiter // mails per address
	chat     *limiter // messages per user
	duel     *limiter // live-series invites per user
	feedback *limiter // feedback messages per user
	locks    sync.Map // user id -> *sync.Mutex
	seen     sync.Map // user id -> time.Time of the last "last seen" write
	hub      *hub     // WebSocket connections held by this instance
	// seasonRolled is the latest ranked season whose reset this instance has run.
	seasonRolled atomic.Int64
	top          topCache // Home's top ten, shared by this server's clients
	log          *slog.Logger
}

func New(st store.Store, cfg Config, log *slog.Logger) *Server {
	s := &Server{st: st, cfg: cfg, origins: map[string]bool{}, log: log,
		general:  newLimiter(15, 40),       // requests/second per IP
		authIP:   newLimiter(1.0/6.0, 10),  // burst 10, then 10/minute
		authAcc:  newLimiter(1.0/30.0, 8),  // burst 8 per account, then 2/minute: slows password guessing
		mailTo:   newLimiter(1.0/300.0, 3), // burst 3 per address, then one every 5 minutes
		chat:     newLimiter(1, 10),        // burst 10, then 1/second
		duel:     newLimiter(1.0/5.0, 3),   // burst 3, then one every 5 seconds
		feedback: newLimiter(1.0/600.0, 5), // burst 5, then one every 10 minutes
	}
	if s.cfg.Bus == nil {
		s.cfg.Bus = bus.NewLocal()
	}
	s.hub = &hub{conns: map[string]map[*wsConn]bool{}}
	s.cfg.Bus.Listen(s.hub.wakeLocal)
	// Every limit is common to all instances, so running more of them does
	// not multiply what one address or one account may do.
	s.general.share(s.cfg.Bus, "req")
	s.authIP.share(s.cfg.Bus, "ip")
	s.authAcc.share(s.cfg.Bus, "acc")
	s.mailTo.share(s.cfg.Bus, "mail")
	s.chat.share(s.cfg.Bus, "chat")
	s.duel.share(s.cfg.Bus, "duel")
	s.feedback.share(s.cfg.Bus, "feedback")
	trustProxy.Store(cfg.TrustProxy)
	for _, o := range cfg.AllowedOrigins {
		s.origins[o] = true
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/meta", s.meta)
	mux.HandleFunc("POST /api/auth/signup", s.signup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/guest", s.guest)
	mux.HandleFunc("POST /api/auth/claim", s.auth(s.claim))
	mux.HandleFunc("GET /api/daily", s.auth(s.daily))
	mux.HandleFunc("POST /api/auth/verify", s.verifyEmail)
	mux.HandleFunc("POST /api/auth/forgot", s.forgot)
	mux.HandleFunc("POST /api/auth/reset", s.resetPassword)
	mux.HandleFunc("GET /api/ws", s.ws)
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/profile", s.auth(s.profile))
	mux.HandleFunc("GET /api/history", s.auth(s.history))
	mux.HandleFunc("POST /api/feedback", s.auth(s.sendFeedback))
	mux.HandleFunc("POST /api/feedback/later", s.auth(s.feedbackLater))
	mux.HandleFunc("POST /api/achievements/seen", s.auth(s.achievementsSeen))
	mux.HandleFunc("GET /api/plan", s.auth(s.getPlan))
	mux.HandleFunc("POST /api/plan", s.auth(s.setPlan))
	mux.HandleFunc("POST /api/profile/password", s.auth(s.changePassword))
	mux.HandleFunc("POST /api/profile/email", s.auth(s.changeEmail))
	mux.HandleFunc("POST /api/profile/resend", s.auth(s.resendVerify))
	mux.HandleFunc("POST /api/profile/delete", s.auth(s.deleteAccount))
	mux.HandleFunc("GET /api/friends", s.auth(s.friends))
	mux.HandleFunc("POST /api/friends/request", s.auth(s.friendRequest))
	mux.HandleFunc("POST /api/friends/respond", s.auth(s.friendRespond))
	mux.HandleFunc("POST /api/friends/remove", s.auth(s.friendRemove))
	mux.HandleFunc("GET /api/chat", s.auth(s.chatList))
	mux.HandleFunc("POST /api/chat", s.auth(s.chatSend))
	mux.HandleFunc("GET /api/ranked", s.auth(s.rankedGet))
	mux.HandleFunc("GET /api/ranking", s.auth(s.ranking))
	mux.HandleFunc("POST /api/ranked/queue", s.auth(s.rankedJoin))
	mux.HandleFunc("POST /api/ranked/cancel", s.auth(s.rankedCancel))
	mux.HandleFunc("POST /api/party/create", s.auth(s.partyCreate))
	mux.HandleFunc("POST /api/party/invite", s.auth(s.partyInvite))
	mux.HandleFunc("POST /api/party/join", s.auth(s.partyJoin))
	mux.HandleFunc("POST /api/party/leave", s.auth(s.partyLeave))
	mux.HandleFunc("POST /api/party/start", s.auth(s.partyStart))
	mux.HandleFunc("GET /api/lobby", s.auth(s.lobbyGet))
	mux.HandleFunc("POST /api/lobby/action", s.auth(s.lobbyAction))
	mux.HandleFunc("POST /api/lobby/order", s.auth(s.lobbyOrder))
	mux.HandleFunc("POST /api/lobby/leave", s.auth(s.lobbyLeave))
	mux.HandleFunc("GET /api/series", s.auth(s.seriesGet))
	mux.HandleFunc("POST /api/series/invite", s.auth(s.seriesInvite))
	mux.HandleFunc("POST /api/series/respond", s.auth(s.seriesRespond))
	mux.HandleFunc("POST /api/series/cancel", s.auth(s.seriesCancel))
	mux.HandleFunc("POST /api/series/leave", s.auth(s.seriesLeave))
	mux.HandleFunc("POST /api/series/action", s.auth(s.seriesAction))
	mux.HandleFunc("POST /api/series/order", s.auth(s.seriesOrder))
	mux.HandleFunc("GET /api/run", s.auth(s.getRun))
	mux.HandleFunc("POST /api/run", s.auth(s.newRun))
	mux.HandleFunc("POST /api/run/action", s.auth(s.action))
	mux.HandleFunc("GET /api/match", s.auth(s.getMatch))
	mux.HandleFunc("POST /api/match/start", s.auth(s.startMatch))
	mux.HandleFunc("POST /api/match/second-half", s.auth(s.secondHalf))
	return s.recoverer(s.headers(s.rateLimit(s.originCheck(mux))))
}

// ---- middleware ----

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "err", v, "path", r.URL.Path)
				writeErr(w, 500, "Internal error.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// trustProxy is process-wide (clientIP has no server at hand); atomic, so
// servers created concurrently, as in tests, do not race on it.
var trustProxy atomic.Bool

func clientIP(r *http.Request) string {
	// RemoteAddr by default: forwarded headers are client-controlled. Behind a
	// trusted proxy (TRUST_PROXY=true) the last X-Forwarded-For entry is the one
	// the proxy itself appended, so earlier, client-supplied entries are ignored.
	if trustProxy.Load() {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.general.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeErr(w, http.StatusTooManyRequests, "Too many requests.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originCheck blocks cross-site state changes. Browsers always send Origin on
// POST, so a missing or foreign Origin is rejected. Combined with the
// SameSite=Strict cookie this closes CSRF without a token.
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !s.origins[r.Header.Get("Origin")] {
				writeErr(w, http.StatusForbidden, "Origin not allowed.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type authed func(w http.ResponseWriter, r *http.Request, sid string)

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// sessionID resolves the session cookie to the id of the signed-in user.
func (s *Server) sessionID(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || len(c.Value) != base64.RawURLEncoding.EncodedLen(tokenLength) {
		return "", false
	}
	uid, err := s.st.SessionUser(r.Context(), hashToken(c.Value))
	if err != nil {
		return "", false
	}
	return uid, true
}

// auth resolves the signed-in user and serialises all requests of that user, so two
// parallel "buy" requests cannot both spend the same budget. The store's
// version check covers the same race across several server instances.
func (s *Server) auth(next authed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), reqTimeout)
		defer cancel()
		r = r.WithContext(ctx)
		sid, ok := s.sessionID(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Please sign in.")
			return
		}
		mu, _ := s.locks.LoadOrStore(sid, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
		s.touch(ctx, sid)
		if r.Method != http.MethodGet && !guestAllowed(r.URL.Path) {
			if u, err := s.st.UserByID(ctx, sid); err != nil || u.Guest {
				writeErr(w, http.StatusForbidden, "Create an account to play with other managers. Your season comes with you.")
				return
			}
		}
		if r.Method == http.MethodGet || !social(r.URL.Path) {
			next(w, r, sid)
			return
		}
		// A change others can see: wake their live connections, on every
		// instance. The circle is read before too, so a removed friend still hears of it.
		before := s.circle(ctx, sid)
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next(sw, r, sid)
		if sw.code < 400 {
			go func() {
				bg, done := context.WithTimeout(context.Background(), 3*time.Second)
				defer done()
				s.cfg.Bus.Publish(append(before, s.circle(bg, sid)...))
			}()
		}
	}
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// decode reads a small, strictly-typed JSON body: wrong content type, unknown
// fields, trailing data and oversized bodies are all rejected.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeMax(w, r, v, maxBody)
}

// decodeMax is decode with a larger body limit, for the few requests that carry text.
func decodeMax(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json.")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		writeErr(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	return true
}

// fail maps errors to responses without leaking internals.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var re game.ErrRule
	switch {
	case errors.As(err, &re):
		writeErr(w, http.StatusUnprocessableEntity, re.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "You have no season in progress.")
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "State changed. Reload.")
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		writeErr(w, http.StatusInternalServerError, "Internal error.")
	}
}

// loadRun returns the user's season, or ErrNotFound when it refers to players
// that have since left the pool, so the client simply starts a new one.
func (s *Server) loadRun(ctx context.Context, owner string) (*game.Run, error) {
	run, err := s.st.GetRun(ctx, owner)
	if err != nil {
		return nil, err
	}
	// A season from before the league (no table) cannot continue either, nor
	// yesterday's daily challenge: the day's table closed at midnight.
	if run.Stale() || run.League == nil || (strings.HasPrefix(owner, "daily:") && run.Daily != today()) {
		return nil, store.ErrNotFound
	}
	return run, nil
}

// ---- handlers ----

func (s *Server) meta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"players": game.Players, "synergies": game.Synergies, "tactics": game.Tactics, "gkLine": game.GKLine,
		"halfTicks": game.HalfTicks, "benchSize": game.BenchSize, "rerollCost": game.RerollCost, "baseIncome": game.BaseIncome, "interestStep": game.InterestStep, "interestMax": game.InterestMax, "xpCost": game.XPCost,
		"augments": game.Augments, "augmentRounds": game.AugmentRounds, "items": game.Items, "itemEvery": game.ItemEvery, "penaltyPrize": game.PenaltyPrize, "penaltyRounds": lobbyWarmup,
		"storage": s.st.Name(),
	})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, sid string) {
	run, err := s.loadRun(r.Context(), runOwner(r, sid))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, run.View())
}

func (s *Server) newRun(w http.ResponseWriter, r *http.Request, sid string) {
	if runOwner(r, sid) != sid {
		run, err := s.newDaily(r, sid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, 201, run.View())
		return
	}
	run, err := game.NewRun(sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Read the stored season directly: a stale one is still replaced in place.
	if old, err := s.st.GetRun(r.Context(), sid); err == nil {
		run.Version = old.Version
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SaveRun(r.Context(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 201, run.View())
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, sid string) {
	var a game.Action
	if !decode(w, r, &a) {
		return
	}
	run, err := s.loadRun(r.Context(), runOwner(r, sid))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := run.Apply(a); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SaveRun(r.Context(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, run.View())
}

type halfResponse struct {
	Half   int                `json:"half"`
	Roster []game.RosterEntry `json:"roster"`
	Frames []game.Frame       `json:"frames"`
	Events []game.Event       `json:"events"`
	Score  [2]int             `json:"score"`
	Stats  [2]game.TeamStats  `json:"stats"`
	Pens   []game.PenKick     `json:"pens,omitempty"`
	Result *game.HistoryEntry `json:"result,omitempty"`
	Names  [2]string          `json:"names"`
	Run    game.RunView       `json:"run"`
	// Achievements this match unlocked.
	Unlocked []unlockedView `json:"unlocked,omitempty"`
}

func (s *Server) startMatch(w http.ResponseWriter, r *http.Request, sid string) {
	run, err := s.loadRun(r.Context(), runOwner(r, sid))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := run.StartMatch()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SaveRun(r.Context(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, halfResponse{Half: 1, Roster: res.Roster, Frames: res.Frames, Events: res.Events, Score: res.Score,
		Stats: res.Stats, Names: [2]string{run.Match.My.Name, run.Match.Opp.Name}, Run: run.View()})
}

func (s *Server) getMatch(w http.ResponseWriter, r *http.Request, sid string) {
	run, err := s.loadRun(r.Context(), runOwner(r, sid))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := run.FirstHalf()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, halfResponse{Half: 1, Roster: res.Roster, Frames: res.Frames, Events: res.Events, Score: res.Score,
		Stats: res.Stats, Names: [2]string{run.Match.My.Name, run.Match.Opp.Name}, Run: run.View()})
}

func (s *Server) secondHalf(w http.ResponseWriter, r *http.Request, sid string) {
	var body struct {
		Tactic string `json:"tactic"`
	}
	if !decode(w, r, &body) {
		return
	}
	run, err := s.loadRun(r.Context(), runOwner(r, sid))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var names [2]string
	if run.Match != nil {
		names = [2]string{run.Match.My.Name, run.Match.Opp.Name}
	}
	res, h, err := run.FinishMatch(body.Tactic)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.st.SaveRun(r.Context(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	var unlocked []string
	if h != nil {
		d := map[string]int{}
		matchStats(d, *h)
		// The league is decided on its last matchday, whether or not Europe follows.
		if h.Round == game.MaxRounds && run.Result != "sacked" && run.League != nil {
			d["seasonsSurvived"] = 1
			me := run.League.Mine()
			if run.Result == "champion" {
				d["titles"] = 1
				if me.L == 0 {
					d["unbeatenTitles"] = 1
				}
			}
			if me.W == game.MaxRounds {
				d["perfectSeasons"] = 1
			}
		}
		if run.Phase == "ended" {
			d["seasons"] = 1
			gameStats(d, run)
			cupStats(d, run)
			g := store.GameRecord{UserID: sid, Mode: "solo", Result: run.Result, Of: game.LeagueTeams}
			if run.League != nil {
				g.Place, g.Points = run.League.Position(), run.League.Mine().Pts
			}
			if run.Cup != nil {
				g.Cup = run.Cup.ID + ":" + run.Cup.Out
			}
			if run.Daily != "" && run.League != nil {
				me := run.League.Mine()
				g.Mode, g.Day, g.GD = "daily", run.Daily, me.GF-me.GA
			}
			s.recordGame(r.Context(), g, run)
		}
		unlocked = s.recordStats(r.Context(), sid, d)
	}
	var events []game.Event
	for _, e := range res.Events {
		if e.T > game.HalfTicks || (e.T == game.HalfTicks && e.Type == "kickoff") {
			events = append(events, e)
		}
	}
	writeJSON(w, 200, halfResponse{Half: 2, Roster: res.Roster, Frames: res.Frames[game.HalfTicks:], Events: events, Score: res.Score,
		Stats: res.Stats, Pens: res.Pens, Result: h, Names: names, Run: run.View(), Unlocked: views(unlocked)})
}

// ---- rate limiting ----

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu    sync.Mutex
	rate  float64
	burst float64
	m     map[string]*bucket
	// Set by share: the bucket then lives on the bus, common to all instances.
	bus  bus.Bus
	name string
	// After a failed call the bus is left alone until this moment (unix
	// nanoseconds), so an outage costs one short wait instead of one per request.
	retryAt atomic.Int64
}

const (
	sharedTimeout = 250 * time.Millisecond
	sharedBackoff = 5 * time.Second
)

func (l *limiter) share(b bus.Bus, name string) { l.bus, l.name = b, name }

// shared asks the common bucket. ok is false when there is none or it cannot
// be reached; the caller then falls back to this instance's own bucket, so a
// Redis outage weakens the limit to per-instance instead of removing it.
func (l *limiter) shared(key string, cost float64) (allowed, ok bool) {
	if l.bus == nil || time.Now().UnixNano() < l.retryAt.Load() {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), sharedTimeout)
	defer cancel()
	allowed, err := l.bus.Allow(ctx, l.name, key, l.rate, l.burst, cost)
	if err != nil {
		l.retryAt.Store(time.Now().Add(sharedBackoff).UnixNano())
		return false, false
	}
	return allowed, true
}

func newLimiter(rate, burst float64) *limiter {
	l := &limiter{rate: rate, burst: burst, m: map[string]*bucket{}}
	go func() {
		for range time.Tick(time.Minute) {
			l.mu.Lock()
			for k, b := range l.m {
				if time.Since(b.last) > 10*time.Minute {
					delete(l.m, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// peek reports whether a call to allow would pass, without spending a token.
func (l *limiter) peek(key string) bool {
	if allowed, ok := l.shared(key, 0); ok {
		return allowed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.m[key]
	return !ok || min(l.burst, b.tokens+time.Since(b.last).Seconds()*l.rate) >= 1
}

func (l *limiter) allow(key string) bool {
	if allowed, ok := l.shared(key, 1); ok {
		return allowed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.m[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.m[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
