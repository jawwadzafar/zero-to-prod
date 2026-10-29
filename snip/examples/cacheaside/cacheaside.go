// Package cacheaside puts a Redis cache in front of one slow function
// (chapter 6.7): cache-aside reads, TTLs with jitter, invalidation on write,
// a broken cache treated as a miss, and request coalescing against stampedes.
//
// Nothing here knows about snip; chapter 6.7 applies the same pattern to
// snip's redirects afterwards.
package cacheaside

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// ErrNotFound is returned when the source has no value for a key.
var ErrNotFound = errors.New("not found")

// Source stands in for the slow, authoritative thing you cache: a database
// query, a call to another service, an expensive computation. It counts how
// often it is asked, so you can see the cache working.
type Source struct {
	Delay time.Duration // how long each lookup takes

	calls atomic.Int64
	mu    sync.Mutex
	data  map[string]string
}

// NewSource returns a source whose every lookup takes delay.
func NewSource(delay time.Duration) *Source {
	return &Source{Delay: delay, data: map[string]string{}}
}

// Get looks key up, slowly.
func (s *Source) Get(ctx context.Context, key string) (string, error) {
	s.calls.Add(1)
	select {
	case <-time.After(s.Delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Put changes the truth. Callers must also invalidate the cache.
func (s *Source) Put(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Calls reports how many lookups reached the source.
func (s *Source) Calls() int64 { return s.calls.Load() }

// Cache is cache-aside in front of a Source.
type Cache struct {
	Redis    *redis.Client
	Source   *Source
	Prefix   string        // namespaces this cache's keys, e.g. "price:"
	TTL      time.Duration // how long a copy may live
	Coalesce bool          // share one source lookup between concurrent misses

	group singleflight.Group
}

// Get returns the value for key and whether the cache answered.
func (c *Cache) Get(ctx context.Context, key string) (value string, hit bool, err error) {
	v, err := c.Redis.Get(ctx, c.Prefix+key).Result()
	if err == nil {
		return v, true, nil // hit
	}
	cacheBroken := !errors.Is(err, redis.Nil) // redis.Nil is a plain miss; anything else means Redis is unwell

	load := func() (any, error) {
		v, err := c.Source.Get(ctx, key)
		if err == nil && !cacheBroken {
			_ = c.Redis.Set(ctx, c.Prefix+key, v, c.jitteredTTL()).Err() // best effort: a failed fill only costs a future miss
		}
		return v, err
	}
	var res any
	if c.Coalesce {
		res, err, _ = c.group.Do(key, load) // concurrent misses for the same key wait for one load
	} else {
		res, err = load()
	}
	if err != nil {
		return "", false, err
	}
	return res.(string), false, nil
}

// Put writes the truth first, then deletes the cached copy. The other order
// leaves a gap in which a reader can re-cache the old value.
func (c *Cache) Put(ctx context.Context, key, value string) error {
	c.Source.Put(key, value)
	return c.Redis.Del(ctx, c.Prefix+key).Err()
}

// jitteredTTL adds up to 10% at random, so keys cached together don't all
// expire together.
func (c *Cache) jitteredTTL() time.Duration {
	if c.TTL <= 0 {
		return 0
	}
	return c.TTL + time.Duration(rand.Int64N(int64(c.TTL)/10+1))
}
