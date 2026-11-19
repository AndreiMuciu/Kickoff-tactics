package api

import (
	"context"
	"errors"
	"testing"
)

// fakeBus is a shared bucket that allows `left` more calls, or fails.
type fakeBus struct {
	left  int
	calls int
	err   error
}

func (f *fakeBus) Publish([]string)      {}
func (f *fakeBus) Listen(func([]string)) {}
func (f *fakeBus) Name() string          { return "fake" }
func (f *fakeBus) Close() error          { return nil }
func (f *fakeBus) Allow(_ context.Context, _, _ string, _, _, cost float64) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	if f.left <= 0 {
		return false, nil
	}
	if cost > 0 {
		f.left--
	}
	return true, nil
}

// Two instances sharing one bucket allow as much together as one alone.
func TestLimiterSharedAcrossInstances(t *testing.T) {
	b := &fakeBus{left: 3}
	one, two := newLimiter(1, 100), newLimiter(1, 100)
	one.share(b, "req")
	two.share(b, "req")
	allowed := 0
	for i := 0; i < 5; i++ {
		if one.allow("ip") {
			allowed++
		}
		if two.allow("ip") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("allowed %d requests through two instances, want the shared 3", allowed)
	}
}

// When the bus fails, the instance's own bucket takes over and the bus is not
// asked again on every request.
func TestLimiterFallsBackWhenBusFails(t *testing.T) {
	b := &fakeBus{err: errors.New("down")}
	l := newLimiter(0.001, 2)
	l.share(b, "req")
	got := []bool{l.allow("ip"), l.allow("ip"), l.allow("ip")}
	if !got[0] || !got[1] || got[2] {
		t.Fatalf("local fallback gave %v, want [true true false]", got)
	}
	if b.calls != 1 {
		t.Fatalf("bus was asked %d times during the outage, want 1", b.calls)
	}
}
