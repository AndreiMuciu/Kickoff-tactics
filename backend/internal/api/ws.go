package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// The WebSocket carries the same answers as the GET routes, pushed instead of
// asked for. A client "watches" a route; the server re-runs that route's
// handler when something may have changed (a wake-up from the bus, or the
// route's own interval, since live games move with the clock) and sends the
// answer when it differs from the last one. Nothing new is exposed: every
// watched route is one the signed-in user could have polled.

const (
	wsMaxPerUser  = 6
	wsMaxWatches  = 8
	wsReadLimit   = 2048
	wsWriteWait   = 5 * time.Second
	wsPingEvery   = 25 * time.Second
	wsRecheck     = 30 * time.Second // session validity and presence
	wsMaxMessages = 30               // client messages per 10 seconds
)

type watchKind struct {
	every  time.Duration
	always bool // send on every interval: the answer carries the server clock
}

var watchIDRE = regexp.MustCompile(`^[a-z0-9_-]{1,24}$`)

type watch struct {
	id      string
	path    string
	u       *url.URL
	kind    watchKind
	handler authed
	game    bool // a live game followed by id: the server tracks the frame offset
	round   int
	frames  int
	next    time.Time
	last    [32]byte
	sent    bool
}

type wsConn struct {
	uid  string
	wake chan struct{}
}

type hub struct {
	mu    sync.Mutex
	conns map[string]map[*wsConn]bool
}

func (h *hub) add(c *wsConn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.conns[c.uid]) >= wsMaxPerUser {
		return false
	}
	if h.conns[c.uid] == nil {
		h.conns[c.uid] = map[*wsConn]bool{}
	}
	h.conns[c.uid][c] = true
	return true
}

func (h *hub) remove(c *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[c.uid], c)
	if len(h.conns[c.uid]) == 0 {
		delete(h.conns, c.uid)
	}
}

// wakeLocal nudges the connections these users hold on this instance.
func (h *hub) wakeLocal(ids []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range ids {
		for c := range h.conns[id] {
			select {
			case c.wake <- struct{}{}:
			default: // already pending
			}
		}
	}
}

// watchRoutes lists what may be watched. Live games are followed every second.
func (s *Server) watchRoutes() map[string]struct {
	h    authed
	kind watchKind
} {
	type e = struct {
		h    authed
		kind watchKind
	}
	return map[string]e{
		"/api/lobby":   {s.lobbyGet, watchKind{time.Second, true}},
		"/api/series":  {s.seriesGet, watchKind{time.Second, false}},
		"/api/ranked":  {s.rankedGet, watchKind{time.Second, false}},
		"/api/friends": {s.friends, watchKind{5 * time.Second, false}},
		"/api/chat":    {s.chatList, watchKind{5 * time.Second, false}},
		"/api/ranking": {s.ranking, watchKind{15 * time.Second, false}},
	}
}

type recorder struct {
	h    http.Header
	code int
	buf  bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.h }
func (r *recorder) WriteHeader(code int)        { r.code = code }
func (r *recorder) Write(b []byte) (int, error) { return r.buf.Write(b) }

// runWatch executes the watched route exactly as a GET from this user would.
func (s *Server) runWatch(ctx context.Context, uid string, wt *watch) (int, []byte) {
	u := *wt.u
	if wt.game {
		q := u.Query()
		q.Set("from", strconv.Itoa(wt.frames))
		u.RawQuery = q.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, reqTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 500, nil
	}
	rec := &recorder{h: http.Header{}, code: 200}
	mu, _ := s.locks.LoadOrStore(uid, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	func() {
		defer mu.(*sync.Mutex).Unlock()
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic in watch", "err", v, "path", wt.path)
				rec.code = 500
			}
		}()
		wt.handler(rec, req, uid)
	}()
	return rec.code, bytes.TrimSpace(rec.buf.Bytes())
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	// A WebSocket handshake is a GET, which the origin middleware lets through,
	// and browsers do not apply the same-origin policy to it. Without this
	// check any website could open a socket with the visitor's cookie.
	if !s.origins[r.Header.Get("Origin")] {
		writeErr(w, http.StatusForbidden, "Origin not allowed.")
		return
	}
	uid, ok := s.sessionID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Please sign in.")
		return
	}
	session := s.currentSession(r)
	conn := &wsConn{uid: uid, wake: make(chan struct{}, 1)}
	if !s.hub.add(conn) {
		writeErr(w, http.StatusTooManyRequests, "Too many open connections.")
		return
	}
	defer s.hub.remove(conn)
	// The origin was checked above against the exact allow-list.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(wsReadLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	type clientMsg struct {
		Op    string `json:"op"`
		ID    string `json:"id"`
		Path  string `json:"path"`
		Round int    `json:"round"`
		From  int    `json:"from"`
	}
	msgs := make(chan clientMsg, 8)
	go func() {
		defer cancel()
		window, count := time.Now(), 0
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if time.Since(window) > 10*time.Second {
				window, count = time.Now(), 0
			}
			if count++; count > wsMaxMessages {
				c.Close(websocket.StatusPolicyViolation, "too many messages")
				return
			}
			var m clientMsg
			if json.Unmarshal(data, &m) != nil {
				c.Close(websocket.StatusUnsupportedData, "bad message")
				return
			}
			select {
			case msgs <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	routes := s.watchRoutes()
	watches := map[string]*watch{}
	send := func(wt *watch) bool {
		code, body := s.runWatch(ctx, uid, wt)
		if code == http.StatusUnauthorized {
			c.Close(websocket.StatusPolicyViolation, "signed out")
			return false
		}
		if len(body) == 0 {
			body = []byte("null")
		}
		if wt.game && code == 200 {
			// Mirror the client's bookkeeping, so the next answer starts where this one ended.
			var g struct {
				Round  int               `json:"round"`
				From   int               `json:"from"`
				Roster []json.RawMessage `json:"roster"`
				Frames []json.RawMessage `json:"frames"`
			}
			if json.Unmarshal(body, &g) == nil {
				if g.Round != wt.round {
					wt.round, wt.frames = g.Round, 0
				}
				if len(g.Roster) > 0 && g.From == wt.frames {
					wt.frames += len(g.Frames)
				}
			}
		}
		sum := sha256.Sum256(append([]byte(strconv.Itoa(code)+":"), body...))
		if wt.sent && !wt.kind.always && sum == wt.last {
			return true
		}
		wt.last, wt.sent = sum, true
		out := make([]byte, 0, len(body)+64)
		out = append(out, `{"id":"`+wt.id+`","status":`+strconv.Itoa(code)+`,"data":`...)
		out = append(out, body...)
		out = append(out, '}')
		wctx, wcancel := context.WithTimeout(ctx, wsWriteWait)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, out) == nil
	}

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	ping := time.NewTicker(wsPingEvery)
	defer ping.Stop()
	recheck := time.NewTicker(wsRecheck)
	defer recheck.Stop()
	s.touch(ctx, uid)

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-msgs:
			switch m.Op {
			case "unwatch":
				delete(watches, m.ID)
			case "watch":
				u, err := url.Parse(m.Path)
				if err != nil || !watchIDRE.MatchString(m.ID) || len(m.Path) > 300 || u.Host != "" || u.Scheme != "" || u.Fragment != "" {
					c.Close(websocket.StatusPolicyViolation, "bad watch")
					return
				}
				rt, known := routes[u.Path]
				if !known || (len(watches) >= wsMaxWatches && watches[m.ID] == nil) {
					c.Close(websocket.StatusPolicyViolation, "bad watch")
					return
				}
				wt := &watch{id: m.ID, path: u.Path, u: u, kind: rt.kind, handler: rt.h}
				if id := u.Query().Get("id"); id != "" && (u.Path == "/api/lobby" || u.Path == "/api/series") {
					wt.game, wt.kind.always = true, true
					wt.round, wt.frames = m.Round, max(0, m.From)
				}
				watches[m.ID] = wt
				if !send(wt) {
					return
				}
				wt.next = time.Now().Add(wt.kind.every)
			default:
				c.Close(websocket.StatusPolicyViolation, "bad message")
				return
			}
		case <-conn.wake:
			// Something changed for this user, here or on another instance.
			now := time.Now()
			for _, wt := range watches {
				if wt.game {
					continue // live games already move every second
				}
				if !send(wt) {
					return
				}
				wt.next = now.Add(wt.kind.every)
			}
		case now := <-tick.C:
			for _, wt := range watches {
				if now.Before(wt.next) {
					continue
				}
				if !send(wt) {
					return
				}
				wt.next = now.Add(wt.kind.every)
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, wsWriteWait)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		case <-recheck.C:
			// A session that was ended elsewhere (log out, password change) closes the socket.
			if owner, err := s.st.SessionUser(ctx, session); err != nil || owner != uid {
				c.Close(websocket.StatusPolicyViolation, "signed out")
				return
			}
			s.touch(ctx, uid)
		}
	}
}

// touch records presence, at most once every 30 seconds per user.
func (s *Server) touch(ctx context.Context, uid string) {
	now := time.Now()
	if last, ok := s.seen.Load(uid); !ok || now.Sub(last.(time.Time)) > 30*time.Second {
		s.seen.Store(uid, now)
		_ = s.st.TouchUser(ctx, uid, now)
	}
}

// ---- who to wake ----

// social reports whether a state-changing route can alter what other managers see on their home page.
func social(path string) bool {
	switch {
	case strings.HasPrefix(path, "/api/friends/"), path == "/api/chat", strings.HasPrefix(path, "/api/party/"),
		strings.HasPrefix(path, "/api/ranked/"), path == "/api/lobby/leave",
		path == "/api/series/invite", path == "/api/series/respond", path == "/api/series/cancel", path == "/api/series/leave":
		return true
	}
	return false
}

// circle is the caller plus everyone whose screen his action may change:
// friends (accepted or pending) and the managers in his lobby or series.
func (s *Server) circle(ctx context.Context, uid string) []string {
	set := map[string]bool{uid: true}
	if fs, err := s.st.ListFriendships(ctx, uid); err == nil {
		for _, f := range fs {
			set[other(f, uid)] = true
		}
	}
	if lb, err := s.st.ActiveLobby(ctx, uid); err == nil && lb != nil {
		for _, id := range append(append([]string{}, lb.Users...), lb.Invited...) {
			set[id] = true
		}
	}
	if sr, err := s.st.ActiveSeries(ctx, uid); err == nil && sr != nil {
		set[sr.A], set[sr.B] = true, true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
