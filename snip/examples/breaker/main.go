// A circuit breaker, watched live (chapter 8.4).
//
// A fake dependency is healthy, then fails for a while, then recovers. The
// breaker stops calling it while it's failing (failing fast instead of
// waiting), then carefully tests whether it has recovered.
//
//	go run ./examples/breaker
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// State of the breaker.
type State int

const (
	Closed   State = iota // normal: calls go through
	Open                  // tripped: calls fail immediately
	HalfOpen              // testing: one trial call allowed
)

func (s State) String() string { return [...]string{"CLOSED", "OPEN", "HALF-OPEN"}[s] }

var ErrOpen = errors.New("circuit open: failing fast")

// Breaker trips after `threshold` consecutive failures and stays open for
// `cooldown` before allowing a single trial call.
type Breaker struct {
	mu        sync.Mutex
	state     State
	failures  int
	threshold int
	cooldown  time.Duration
	openedAt  time.Time
	trials    int // how many half-open trial calls have been made (for the demo output)
}

func (b *Breaker) Call(op func() error) error {
	b.mu.Lock()
	if b.state == Open {
		if time.Since(b.openedAt) < b.cooldown {
			b.mu.Unlock()
			return ErrOpen // don't even try: protect ourselves and the dependency
		}
		b.state = HalfOpen // cooldown over: allow one trial
		b.trials++
	}
	b.mu.Unlock()

	err := op()

	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.state, b.failures = Closed, 0 // success closes the circuit
		return nil
	}
	b.failures++
	if b.state == HalfOpen || b.failures >= b.threshold {
		b.state, b.openedAt = Open, time.Now() // trip (or re-trip after a failed trial)
	}
	return err
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func main() {
	start := time.Now()
	// The dependency is down between 0.3s and 1.5s after start, and slow when down.
	dependency := func() error {
		t := time.Since(start)
		if t > 300*time.Millisecond && t < 1500*time.Millisecond {
			time.Sleep(100 * time.Millisecond) // failing calls are also slow (timeouts)
			return errors.New("dependency error")
		}
		return nil
	}

	b := &Breaker{threshold: 3, cooldown: 400 * time.Millisecond}
	var calls, fastFails int
	for time.Since(start) < 2*time.Second {
		trialsBefore := b.trials
		err := b.Call(func() error { calls++; return dependency() })
		trial := b.trials > trialsBefore
		var result string
		switch {
		case trial && err == nil:
			result = "TRIAL CALL → ok, circuit closes"
		case trial:
			result = "TRIAL CALL → still failing, circuit re-opens"
		case errors.Is(err, ErrOpen):
			result = "failed fast (not called)"
			fastFails++
		case err != nil:
			result = "error: " + err.Error()
		default:
			result = "ok"
		}
		fmt.Printf("%5dms  breaker=%-9s  %s\n", time.Since(start).Milliseconds(), b.State(), result)
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("\nthe dependency was actually called %d times; %d requests failed fast without calling it\n", calls, fastFails)
}
