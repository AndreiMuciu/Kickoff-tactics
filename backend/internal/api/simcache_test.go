package api

import (
	"sync"
	"sync/atomic"
	"testing"

	"footy/internal/game"
)

// Many viewers asking for the same match at once cost one simulation, and
// they all get the same result as a direct simulation would give.
func TestSimCacheSimulatesOnce(t *testing.T) {
	c := &simCache{m: map[string]*simEntry{}}
	var runs atomic.Int32
	seed, _, _ := game.NewSeed()
	squads := [2]game.Lineup{game.WarmupClub(game.NewRNG(1), 3), game.WarmupClub(game.NewRNG(2), 3)}
	orders := [2]string{"balanced", "press"}
	var wg sync.WaitGroup
	results := make([]*game.SimResult, 32)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = c.get(simKey("m", seed, squads, orders, 2), func() (*game.SimResult, error) {
				runs.Add(1)
				return game.SimulateSeed(seed, squads, orders, 2)
			})
		}(i)
	}
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("%d simulations for one match", runs.Load())
	}
	direct, _ := game.SimulateSeed(seed, squads, orders, 2)
	for _, r := range results {
		if r != results[0] || r.Score != direct.Score || len(r.Frames) != len(direct.Frames) {
			t.Fatal("viewers got different results")
		}
	}
	// A different order is a different match.
	other := [2]string{"balanced", "park_bus"}
	if simKey("m", seed, squads, orders, 2) == simKey("m", seed, squads, other, 2) {
		t.Fatal("orders do not change the key")
	}
	// The cache stays bounded.
	for i := 0; i < simCacheSize+10; i++ {
		_, _ = c.get(simKey("m", seed, squads, orders, i), func() (*game.SimResult, error) { return &game.SimResult{}, nil })
	}
	if len(c.m) > simCacheSize || len(c.order) > simCacheSize {
		t.Fatalf("cache holds %d entries", len(c.m))
	}
}
