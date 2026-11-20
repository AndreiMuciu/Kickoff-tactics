// Package store persists accounts, sessions, friendships, chat and seasons.
// Two implementations: in-memory (default, nothing to install) and MongoDB
// (set MONGODB_URI).
package store

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"footy/internal/game"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict means a uniqueness or version check failed.
	ErrConflict = errors.New("conflict")
	// ErrEmailTaken means another account already uses that address.
	ErrEmailTaken = errors.New("email taken")
)

const (
	SessionTTL = 7 * 24 * time.Hour
	MessageTTL = 30 * 24 * time.Hour
)

type User struct {
	ID        string `bson:"_id"`
	Name      string
	NameKey   string // lower-case, unique
	PassHash  string
	CreatedAt time.Time
	LastSeen  time.Time
	LP        int // ranked points; everyone starts at 0 and it never goes below 0
	Games     int // ranked lobbies finished
	// Email is optional for accounts created before it existed. EmailKey is the
	// lower-case form and unique; it is left out of the document when empty so
	// the unique index ignores accounts without one.
	Email    string `bson:"email,omitempty"`
	EmailKey string `bson:"emailkey,omitempty"`
	Verified bool   // the address in Email has been confirmed by its owner
	// Plan is the manager's team plan: the players (ids) he wants to end up with.
	Plan []string `bson:"plan,omitempty"`
	// Stats are plain counters (matches, wins, gf, ga, seasons, titles, lobbies,
	// lobbyWins, lobbyTop4, placeSum, ranked, series, seriesWins).
	Stats map[string]int
}

// Token is a single-use secret mailed to a user: "verify" confirms an address,
// "reset" allows choosing a new password. Stored by the SHA-256 of the secret.
type Token struct {
	ID       string `bson:"_id"` // hash of the secret
	UserID   string
	Kind     string
	EmailKey string // the address it was sent to
	Expires  time.Time
}

// Friendship is one document per pair of users. ID is PairID(A, B).
type Friendship struct {
	ID        string `bson:"_id"`
	A, B      string
	Requester string
	Accepted  bool
	CreatedAt time.Time
}

type Message struct {
	ID     string `bson:"_id"`
	Pair   string
	From   string // user id; empty for system lines
	Text   string
	System bool
	At     time.Time
}

// SeriesResult is one finished match of a series, from A's point of view.
type SeriesResult struct {
	Round  int    `json:"round"`
	Score  [2]int `json:"score"`
	Winner int    `json:"winner"` // 0 = A, 1 = B
	Pens   bool   `json:"pens"`
}

// Series is a head-to-head season between two friends, played live. Both
// start from scratch and meet every round. Its whole timeline is derived from
// RoundStart, so any server instance can answer a poll: nothing about a
// running series lives in process memory. Each manager's club is a separate
// Run document (see api.seriesRunKey), so their moves never collide.
type Series struct {
	ID           string `bson:"_id"`
	A, B         string // user ids; A invited B and plays as team 0
	NameA, NameB string
	Status       string // invited | live | done
	Round        int
	RoundStart   time.Time
	Frozen       bool // squads locked and seed drawn for this round's match
	Seed         string
	Commit       string
	Squads       [2]game.Lineup
	Orders       [2]string // second-half orders, hidden from the opponent
	Settled      bool      // this round's result has been applied to both clubs
	PenTicks     int       // length of this round's penalty shootout in ticks, 0 without one
	LastSquads   [2]game.Lineup
	Results      []SeriesResult
	Winner       int // -1 until the series is decided
	CreatedAt    time.Time
	Version      int64
}

// A Lobby is a ranked game for up to eight managers, bots filling empty seats.
// Every round the survivors are paired off and play each other; losing costs
// patience, and a manager at zero is out. Like a Series, its whole timeline is
// derived from RoundStart and nothing about it lives in process memory.
type LobbySeat struct {
	UserID   string // empty for a bot
	Name     string
	Bot      bool
	Alive    bool
	Place    int // final position, 0 while still in
	Rated    bool
	Patience int
	Streak   int // +n wins or -n defeats in a row, as of the last settled round
	LPChange int
	Rating   int // points when the lobby started: decides which tier's table applies
}

// LobbyPair is one match of a round. With an odd number of survivors the last
// one plays a ghost: a copy of another manager's squad, who is not affected.
type LobbyPair struct {
	A, B     int // seat indexes; A is team 0
	Ghost    bool
	NPC      string // older lobbies' warm-up round: the name of the small club A played instead of a manager
	Shootout bool   // a penalty round: the meeting is settled by penalties alone
	Seed     string
	Commit   string
	Squads   [2]game.Lineup
	Orders   [2]string
	PenTicks int
}

type Lobby struct {
	ID         string   `bson:"_id"`
	Users      []string // the human managers, for lookup
	Status     string   // open (a friends lobby still gathering) | live | done
	Ranked     bool     // false: a normal or friends lobby, no points at stake
	Host       string   // friends lobby: the manager who created it and starts it
	Invited    []string // friends lobby: user ids invited but not yet in
	Seats      []LobbySeat
	Round      int
	RoundStart time.Time
	Frozen     bool
	Settled    bool
	Pairs      []LobbyPair
	PenTicks   int           // longest shootout of the round: everyone waits for it
	Last       []game.Lineup // each seat's squad in its previous match, for scouting
	CreatedAt  time.Time
	Version    int64
}

// Queue is the matchmaking queue of one mode; its ID is the mode.
type QueueEntry struct {
	UserID   string
	Name     string
	JoinedAt time.Time
	SeenAt   time.Time // last poll; a manager who stops polling is dropped
	Rating   int       // points when joining, used to match managers of similar level
}

type Queue struct {
	ID      string `bson:"_id"`
	Entries []QueueEntry
	Version int64
}

// PairID is the order-independent key of two user ids.
func PairID(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + ":" + b
}

type Store interface {
	CreateUser(ctx context.Context, u *User) error // ErrConflict when the name is taken
	UserByName(ctx context.Context, nameKey string) (*User, error)
	UserByID(ctx context.Context, id string) (*User, error)
	UsersByIDs(ctx context.Context, ids []string) (map[string]*User, error)
	TouchUser(ctx context.Context, id string, at time.Time) error
	UserByEmail(ctx context.Context, emailKey string) (*User, error)
	// SetEmail stores a new, unconfirmed address. ErrConflict when another account has it.
	SetEmail(ctx context.Context, id, email, emailKey string) error
	// SetVerified confirms the address, but only if it is still the account's address.
	SetVerified(ctx context.Context, id, emailKey string) error
	SetPassword(ctx context.Context, id, hash string) error
	// SetPlan replaces the manager's team plan.
	SetPlan(ctx context.Context, id string, plan []string) error
	// AddStats adds to the user's counters.
	AddStats(ctx context.Context, id string, delta map[string]int) error

	PutToken(ctx context.Context, t *Token) error
	// TakeToken returns the token and deletes it in one step, so it works once.
	TakeToken(ctx context.Context, hash string) (*Token, error)
	DeleteTokens(ctx context.Context, userID, kind string) error

	// Sessions are stored by the SHA-256 of the token, never the token itself.
	CreateSession(ctx context.Context, tokenHash, userID string) error
	SessionUser(ctx context.Context, tokenHash string) (string, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	// DeleteUserSessions signs the user out everywhere except the session `keep` (may be empty).
	DeleteUserSessions(ctx context.Context, userID, keep string) error

	GetFriendship(ctx context.Context, id string) (*Friendship, error)
	PutFriendship(ctx context.Context, f *Friendship) error
	DeleteFriendship(ctx context.Context, id string) error
	ListFriendships(ctx context.Context, userID string) ([]Friendship, error)

	AddMessage(ctx context.Context, m *Message) error
	// ListMessages returns the newest `limit` messages of a pair, oldest first.
	ListMessages(ctx context.Context, pair string, limit int) ([]Message, error)

	GetSeries(ctx context.Context, id string) (*Series, error)
	// ActiveSeries returns the user's newest series that is still invited or live.
	ActiveSeries(ctx context.Context, userID string) (*Series, error)
	// SaveSeries writes with the same optimistic version rule as SaveRun.
	SaveSeries(ctx context.Context, m *Series) error
	DeleteSeries(ctx context.Context, id string) error

	GetLobby(ctx context.Context, id string) (*Lobby, error)
	// ActiveLobby returns the newest open or live lobby the user sits in or is
	// invited to, if any.
	ActiveLobby(ctx context.Context, userID string) (*Lobby, error)
	SaveLobby(ctx context.Context, l *Lobby) error // optimistic, like SaveRun
	// GetQueue returns the matchmaking queue of a mode ("ranked" or "normal"),
	// empty when none is stored yet. SaveQueue writes it back under q.ID.
	GetQueue(ctx context.Context, mode string) (*Queue, error)
	SaveQueue(ctx context.Context, q *Queue) error // optimistic
	// AddRating adds ranked points to a user (never below zero) and counts one finished lobby.
	AddRating(ctx context.Context, userID string, delta int) error
	// TopRated returns every manager ordered by points, best first, up to n.
	TopRated(ctx context.Context, n int) ([]User, error)
	// RankInfo returns how many managers have more than lp points, and how many managers exist.
	RankInfo(ctx context.Context, lp int) (above, total int, err error)

	GetRun(ctx context.Context, ownerID string) (*game.Run, error)
	// SaveRun writes the run only if the stored version still equals run.Version,
	// then bumps it. Version 0 means "create".
	SaveRun(ctx context.Context, run *game.Run) error

	Name() string
	Close(ctx context.Context) error
}

type memSession struct {
	user string
	at   time.Time
}

type Memory struct {
	mu       sync.Mutex
	users    map[string]User
	byName   map[string]string
	sessions map[string]memSession
	friends  map[string]Friendship
	messages map[string][]Message
	runs     map[string][]byte
	series   map[string]Series
	lobbies  map[string]Lobby
	queues   map[string]Queue
	byEmail  map[string]string
	tokens   map[string]Token
}

func NewMemory() *Memory {
	return &Memory{users: map[string]User{}, byName: map[string]string{}, sessions: map[string]memSession{},
		friends: map[string]Friendship{}, messages: map[string][]Message{}, runs: map[string][]byte{}, series: map[string]Series{}, lobbies: map[string]Lobby{}, queues: map[string]Queue{}, byEmail: map[string]string{}, tokens: map[string]Token{}}
}

func (m *Memory) Name() string { return "memory" }

func (m *Memory) Close(context.Context) error { return nil }

func (m *Memory) CreateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byName[u.NameKey]; ok {
		return ErrConflict
	}
	if u.EmailKey != "" {
		if _, ok := m.byEmail[u.EmailKey]; ok {
			return ErrEmailTaken
		}
		m.byEmail[u.EmailKey] = u.ID
	}
	m.users[u.ID] = *u
	m.byName[u.NameKey] = u.ID
	return nil
}

func (m *Memory) UserByEmail(_ context.Context, key string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byEmail[key]
	if !ok || key == "" {
		return nil, ErrNotFound
	}
	u := m.users[id]
	return &u, nil
}

func (m *Memory) SetEmail(_ context.Context, id, email, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	if owner, taken := m.byEmail[key]; taken && owner != id {
		return ErrEmailTaken
	}
	delete(m.byEmail, u.EmailKey)
	u.Email, u.EmailKey, u.Verified = email, key, false
	m.byEmail[key] = id
	m.users[id] = u
	return nil
}

func (m *Memory) SetVerified(_ context.Context, id, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok || u.EmailKey != key || key == "" {
		return ErrNotFound
	}
	u.Verified = true
	m.users[id] = u
	return nil
}

func (m *Memory) SetPassword(_ context.Context, id, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	u.PassHash = hash
	m.users[id] = u
	return nil
}

func (m *Memory) SetPlan(_ context.Context, id string, plan []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	u.Plan = append([]string{}, plan...)
	m.users[id] = u
	return nil
}

func (m *Memory) AddStats(_ context.Context, id string, delta map[string]int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil
	}
	st := map[string]int{}
	for k, v := range u.Stats {
		st[k] = v
	}
	for k, v := range delta {
		st[k] += v
	}
	u.Stats = st
	m.users[id] = u
	return nil
}

func (m *Memory) PutToken(_ context.Context, t *Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.tokens {
		if now.After(v.Expires) {
			delete(m.tokens, k)
		}
	}
	m.tokens[t.ID] = *t
	return nil
}

func (m *Memory) TakeToken(_ context.Context, hash string) (*Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[hash]
	delete(m.tokens, hash)
	if !ok || time.Now().After(t.Expires) {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (m *Memory) DeleteTokens(_ context.Context, userID, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.tokens {
		if v.UserID == userID && v.Kind == kind {
			delete(m.tokens, k)
		}
	}
	return nil
}

func (m *Memory) DeleteUserSessions(_ context.Context, userID, keep string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.sessions {
		if v.user == userID && k != keep {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *Memory) UserByName(_ context.Context, key string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byName[key]
	if !ok {
		return nil, ErrNotFound
	}
	u := m.users[id]
	return &u, nil
}

func (m *Memory) UserByID(_ context.Context, id string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (m *Memory) UsersByIDs(_ context.Context, ids []string) (map[string]*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]*User{}
	for _, id := range ids {
		if u, ok := m.users[id]; ok {
			out[id] = &u
		}
	}
	return out, nil
}

func (m *Memory) TouchUser(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok {
		u.LastSeen = at
		m.users[id] = u
	}
	return nil
}

func (m *Memory) CreateSession(_ context.Context, h, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, s := range m.sessions {
		if now.Sub(s.at) > SessionTTL {
			delete(m.sessions, k)
		}
	}
	m.sessions[h] = memSession{user: userID, at: now}
	return nil
}

func (m *Memory) SessionUser(_ context.Context, h string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[h]
	if !ok || time.Since(s.at) > SessionTTL {
		return "", ErrNotFound
	}
	return s.user, nil
}

func (m *Memory) DeleteSession(_ context.Context, h string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, h)
	return nil
}

func (m *Memory) GetFriendship(_ context.Context, id string) (*Friendship, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.friends[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &f, nil
}

func (m *Memory) PutFriendship(_ context.Context, f *Friendship) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.friends[f.ID] = *f
	return nil
}

func (m *Memory) DeleteFriendship(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.friends, id)
	return nil
}

func (m *Memory) ListFriendships(_ context.Context, userID string) ([]Friendship, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Friendship
	for _, f := range m.friends {
		if f.A == userID || f.B == userID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) AddMessage(_ context.Context, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := append(m.messages[msg.Pair], *msg)
	if len(list) > 500 {
		list = list[len(list)-500:]
	}
	m.messages[msg.Pair] = list
	return nil
}

func (m *Memory) ListMessages(_ context.Context, pair string, limit int) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.messages[pair]
	if len(list) > limit {
		list = list[len(list)-limit:]
	}
	return append([]Message(nil), list...), nil
}

func (m *Memory) GetRun(_ context.Context, id string) (*game.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	var r game.Run
	if err := bson.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (m *Memory) SaveRun(_ context.Context, run *game.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var stored int64
	if b, ok := m.runs[run.SessionID]; ok {
		var cur game.Run
		if err := bson.Unmarshal(b, &cur); err != nil {
			return err
		}
		stored = cur.Version
	}
	if stored != run.Version {
		return ErrConflict
	}
	next := *run
	next.Version++
	b, err := bson.Marshal(&next)
	if err != nil {
		return err
	}
	m.runs[run.SessionID] = b
	run.Version = next.Version
	return nil
}

func (m *Memory) GetSeries(_ context.Context, id string) (*Series, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.series[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &l, nil
}

func (m *Memory) ActiveSeries(_ context.Context, userID string) (*Series, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *Series
	for _, l := range m.series {
		if (l.A == userID || l.B == userID) && l.Status != "done" && (best == nil || l.CreatedAt.After(best.CreatedAt)) {
			c := l
			best = &c
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	return best, nil
}

func (m *Memory) SaveSeries(_ context.Context, l *Series) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.series[l.ID]; (ok && cur.Version != l.Version) || (!ok && l.Version != 0) {
		return ErrConflict
	}
	l.Version++
	m.series[l.ID] = *l
	return nil
}

func (m *Memory) DeleteSeries(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.series, id)
	return nil
}

// deepCopy clones a stored value through BSON, so callers never share slices
// with the store.
func deepCopy[T any](v T) (T, error) {
	var out T
	b, err := bson.Marshal(&v)
	if err != nil {
		return out, err
	}
	err = bson.Unmarshal(b, &out)
	return out, err
}

func (m *Memory) GetLobby(_ context.Context, id string) (*Lobby, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.lobbies[id]
	if !ok {
		return nil, ErrNotFound
	}
	c, err := deepCopy(l)
	return &c, err
}

func (m *Memory) ActiveLobby(_ context.Context, userID string) (*Lobby, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *Lobby
	for _, l := range m.lobbies {
		if l.Status != "live" && l.Status != "open" {
			continue
		}
		for _, u := range append(append([]string{}, l.Users...), l.Invited...) {
			if u == userID && (best == nil || l.CreatedAt.After(best.CreatedAt)) {
				c := l
				best = &c
			}
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	c, err := deepCopy(*best)
	return &c, err
}

func (m *Memory) SaveLobby(_ context.Context, l *Lobby) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.lobbies[l.ID]; (ok && cur.Version != l.Version) || (!ok && l.Version != 0) {
		return ErrConflict
	}
	c, err := deepCopy(*l)
	if err != nil {
		return err
	}
	c.Version++
	m.lobbies[l.ID] = c
	l.Version = c.Version
	return nil
}

func (m *Memory) GetQueue(_ context.Context, mode string) (*Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, ok := m.queues[mode]
	if !ok {
		return &Queue{ID: mode}, nil
	}
	c, err := deepCopy(q)
	return &c, err
}

func (m *Memory) SaveQueue(_ context.Context, q *Queue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queues[q.ID].Version != q.Version {
		return ErrConflict
	}
	c, err := deepCopy(*q)
	if err != nil {
		return err
	}
	c.Version++
	m.queues[q.ID] = c
	q.Version = c.Version
	return nil
}

func (m *Memory) AddRating(_ context.Context, userID string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[userID]; ok {
		u.LP = max(0, u.LP+delta)
		u.Games++
		m.users[userID] = u
	}
	return nil
}

func (m *Memory) RankInfo(_ context.Context, lp int) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	above := 0
	for _, u := range m.users {
		if u.LP > lp {
			above++
		}
	}
	return above, len(m.users), nil
}

func (m *Memory) TopRated(_ context.Context, n int) ([]User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []User
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LP > out[j].LP || (out[i].LP == out[j].LP && out[i].Name < out[j].Name)
	})
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}
