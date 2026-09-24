// Package ratelimit limits how often one caller may do something, using a
// fixed window: at most Limit actions per Window, counted per key.
package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter decides whether key may act now.
type Limiter interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}

// windowStart returns the start of the current window and time until the next.
func windowStart(now time.Time, window time.Duration) (int64, time.Duration) {
	start := now.Truncate(window)
	return start.Unix(), start.Add(window).Sub(now)
}

// Redis is a Limiter shared by every copy of snip, because the counters live
// in Redis rather than in one process's memory.
type Redis struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
	now    func() time.Time
}

// NewRedis allows limit actions per window per key.
func NewRedis(rdb *redis.Client, limit int, window time.Duration) *Redis {
	return &Redis{rdb: rdb, limit: int64(limit), window: window, now: time.Now}
}

func (l *Redis) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	start, untilNext := windowStart(l.now(), l.window)
	k := "snip:rl:" + key + ":" + strconv.FormatInt(start, 10)
	// INCR and EXPIRE in one round trip. INCR is atomic, so two requests
	// arriving together can never both see the same count.
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, l.window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, err // fail open: a Redis outage shouldn't block every user
	}
	if incr.Val() > l.limit {
		return false, untilNext, nil
	}
	return true, 0, nil
}

// Memory is a single-process Limiter for tests and Redis-less mode.
type Memory struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	counts map[string]int
	start  int64
	now    func() time.Time
}

// NewMemory allows limit actions per window per key, in this process only.
func NewMemory(limit int, window time.Duration) *Memory {
	return &Memory{limit: limit, window: window, counts: map[string]int{}, now: time.Now}
}

func (m *Memory) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	start, untilNext := windowStart(m.now(), m.window)
	if start != m.start { // new window: forget old counts
		m.start = start
		m.counts = map[string]int{}
	}
	m.counts[key]++
	if m.counts[key] > m.limit {
		return false, untilNext, nil
	}
	return true, 0, nil
}
