// Package bus is what several server instances share besides the database:
// a "something changed for these users" signal, so a WebSocket held by one
// instance learns of a change made through another, and the rate limits that
// must hold across all instances. With REDIS_URL set it uses Redis; without,
// it works inside one process, which is all a single instance needs.
package bus

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrLocal tells the caller to use its own in-memory limiter.
var ErrLocal = errors.New("no shared limiter")

type Bus interface {
	// Publish wakes the live connections of these users on every instance.
	Publish(userIDs []string)
	// Listen registers the function that wakes local connections. Call once.
	Listen(fn func(userIDs []string))
	// Allow is a token bucket shared by all instances. cost 0 only asks.
	Allow(ctx context.Context, name, key string, rate, burst, cost float64) (bool, error)
	// Spectate records that `viewer` (shown as `name`) is watching seat `seat`
	// of a lobby right now; seat < 0 means he stopped.
	Spectate(ctx context.Context, lobby, viewer, name string, seat int)
	// Spectators lists who has watched each seat of a lobby in the last few seconds.
	Spectators(ctx context.Context, lobby string) map[int][]string
	Name() string
	Close() error
}

// ---- one process ----

type Local struct {
	fn   func([]string)
	mu   sync.Mutex
	spec map[string]map[string]spectator
}

func NewLocal() *Local { return &Local{spec: map[string]map[string]spectator{}} }

// spectator is one viewer of a lobby: the seat he watches, as of when.
type spectator struct {
	seat int
	name string
	at   time.Time
}

// spectatorTTL is how long a viewer counts as watching after his last poll.
const spectatorTTL = 5 * time.Second

func (l *Local) Spectate(_ context.Context, lobby, viewer, name string, seat int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.spec[lobby] == nil {
		l.spec[lobby] = map[string]spectator{}
	}
	if seat < 0 {
		delete(l.spec[lobby], viewer)
		return
	}
	l.spec[lobby][viewer] = spectator{seat, name, time.Now()}
}

func (l *Local) Spectators(_ context.Context, lobby string) map[int][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[int][]string{}
	for id, sp := range l.spec[lobby] {
		if time.Since(sp.at) > spectatorTTL {
			delete(l.spec[lobby], id)
			continue
		}
		out[sp.seat] = append(out[sp.seat], sp.name)
	}
	return out
}

func (l *Local) Name() string { return "in-process (single instance)" }
func (l *Local) Close() error { return nil }
func (l *Local) Listen(fn func([]string)) {
	l.fn = fn
}
func (l *Local) Publish(ids []string) {
	if l.fn != nil && len(ids) > 0 {
		l.fn(ids)
	}
}
func (l *Local) Allow(context.Context, string, string, float64, float64, float64) (bool, error) {
	return false, ErrLocal
}

// ---- Redis ----

const channel = "footy:wake"

// One atomic step: refill by elapsed time, then take `cost` tokens if a whole
// one is available. The key expires once the bucket would be full again.
var bucket = redis.NewScript(`
local d = redis.call('HMGET', KEYS[1], 't', 'ts')
local rate, burst, now, cost = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4])
local t = tonumber(d[1]) or burst
local ts = tonumber(d[2]) or now
t = math.min(burst, t + math.max(0, now - ts) * rate)
local ok = 0
if t >= 1 then
  ok = 1
  t = t - cost
end
redis.call('HSET', KEYS[1], 't', t, 'ts', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000) + 1000)
return ok`)

type Redis struct {
	c      *redis.Client
	cancel context.CancelFunc
	ctx    context.Context
}

// NewRedis connects and checks the connection. The URL may hold a password; never log it.
func NewRedis(ctx context.Context, url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("invalid REDIS_URL")
	}
	opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = 3*time.Second, 2*time.Second, 2*time.Second
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	bg, cancel := context.WithCancel(context.Background())
	return &Redis{c: c, ctx: bg, cancel: cancel}, nil
}

func (r *Redis) Name() string { return "redis" }

func (r *Redis) Close() error {
	r.cancel()
	return r.c.Close()
}

func (r *Redis) Listen(fn func([]string)) {
	sub := r.c.Subscribe(r.ctx, channel) // reconnects by itself
	go func() {
		defer sub.Close()
		for msg := range sub.Channel() {
			if msg.Payload != "" {
				fn(strings.Split(msg.Payload, ","))
			}
		}
	}()
}

func (r *Redis) Publish(ids []string) {
	if len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Second)
	defer cancel()
	// User ids are hex strings, so a comma is a safe separator.
	_ = r.c.Publish(ctx, channel, strings.Join(ids, ",")).Err()
}

func (r *Redis) Allow(ctx context.Context, name, key string, rate, burst, cost float64) (bool, error) {
	now := float64(time.Now().UnixMilli()) / 1000
	n, err := bucket.Run(ctx, r.c, []string{"footy:rl:" + name + ":" + key}, rate, burst, now, cost).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func specKey(lobby string) string { return "footy:spec:" + lobby }

// Spectate keeps one hash per lobby: viewer -> "seat|unix ms|name".
func (r *Redis) Spectate(ctx context.Context, lobby, viewer, name string, seat int) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if seat < 0 {
		_ = r.c.HDel(ctx, specKey(lobby), viewer).Err()
		return
	}
	v := strconv.Itoa(seat) + "|" + strconv.FormatInt(time.Now().UnixMilli(), 10) + "|" + name
	pipe := r.c.TxPipeline()
	pipe.HSet(ctx, specKey(lobby), viewer, v)
	pipe.Expire(ctx, specKey(lobby), time.Hour)
	_, _ = pipe.Exec(ctx)
}

func (r *Redis) Spectators(ctx context.Context, lobby string) map[int][]string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out := map[int][]string{}
	all, err := r.c.HGetAll(ctx, specKey(lobby)).Result()
	if err != nil {
		return out
	}
	now := time.Now().UnixMilli()
	for _, v := range all {
		parts := strings.SplitN(v, "|", 3)
		if len(parts) != 3 {
			continue
		}
		seat, err1 := strconv.Atoi(parts[0])
		at, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil || now-at > spectatorTTL.Milliseconds() {
			continue
		}
		out[seat] = append(out[seat], parts[2])
	}
	return out
}
