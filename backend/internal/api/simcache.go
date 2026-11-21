package api

import (
	"fmt"
	"hash/fnv"
	"sync"

	"footy/internal/game"
)

// A match is fully determined by its seed, the two frozen squads, the
// half-time orders and how many halves are played, yet every poll of a live
// game (each manager, each spectator, both servers) needs the frames. The
// cache below simulates each match once per server and hands the same result
// to everyone; callers only read it. Concurrent requests for a match being
// simulated wait for that one simulation instead of starting their own.
const simCacheSize = 96 // a few lobbies' worth of current matches; one entry is ~0.3 MB

type simEntry struct {
	done chan struct{}
	res  *game.SimResult
	err  error
}

type simCache struct {
	mu    sync.Mutex
	m     map[string]*simEntry
	order []string // oldest first, for eviction
}

var sims = &simCache{m: map[string]*simEntry{}}

func (c *simCache) get(key string, f func() (*game.SimResult, error)) (*game.SimResult, error) {
	c.mu.Lock()
	if e, ok := c.m[key]; ok {
		c.mu.Unlock()
		<-e.done
		return e.res, e.err
	}
	e := &simEntry{done: make(chan struct{})}
	c.m[key] = e
	c.order = append(c.order, key)
	if len(c.order) > simCacheSize {
		delete(c.m, c.order[0])
		c.order = append(c.order[:0:0], c.order[1:]...)
	}
	c.mu.Unlock()

	e.res, e.err = f()
	if e.err != nil { // never keep a failure
		c.mu.Lock()
		if c.m[key] == e {
			delete(c.m, key)
		}
		c.mu.Unlock()
	}
	close(e.done)
	return e.res, e.err
}

func simKey(kind, seed string, squads [2]game.Lineup, orders [2]string, halves int) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%v", squads)
	return fmt.Sprintf("%s|%s|%d|%s|%s|%x", kind, seed, halves, orders[0], orders[1], h.Sum64())
}

// simulate is game.SimulateSeed through the cache.
func simulate(seed string, squads [2]game.Lineup, orders [2]string, halves int) (*game.SimResult, error) {
	return sims.get(simKey("m", seed, squads, orders, halves), func() (*game.SimResult, error) {
		return game.SimulateSeed(seed, squads, orders, halves)
	})
}

// shootout is game.SimulateShootout through the cache.
func shootout(seed string, squads [2]game.Lineup) (*game.SimResult, error) {
	return sims.get(simKey("p", seed, squads, [2]string{}, 0), func() (*game.SimResult, error) {
		return game.SimulateShootout(seed, squads)
	})
}
