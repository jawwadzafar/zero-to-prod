package cacheaside

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newCache(t *testing.T, coalesce bool) *Cache {
	t.Helper()
	addr := os.Getenv("SNIP_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("SNIP_TEST_REDIS_ADDR not set")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	c := &Cache{
		Redis:    rdb,
		Source:   NewSource(50 * time.Millisecond),
		Prefix:   "cacheaside-test:" + t.Name() + ":" + time.Now().Format("150405.000000") + ":",
		TTL:      time.Minute,
		Coalesce: coalesce,
	}
	c.Source.Put("tea", "3.50")
	return c
}

func TestMissThenHit(t *testing.T) {
	c := newCache(t, false)
	ctx := context.Background()
	if v, hit, err := c.Get(ctx, "tea"); err != nil || hit || v != "3.50" {
		t.Fatalf("first read: %q hit=%v err=%v, want a miss", v, hit, err)
	}
	if v, hit, err := c.Get(ctx, "tea"); err != nil || !hit || v != "3.50" {
		t.Fatalf("second read: %q hit=%v err=%v, want a hit", v, hit, err)
	}
	if got := c.Source.Calls(); got != 1 {
		t.Fatalf("source called %d times, want 1", got)
	}
	ttl, err := c.Redis.TTL(ctx, c.Prefix+"tea").Result()
	if err != nil || ttl <= 0 || ttl > c.TTL+c.TTL/10+time.Second {
		t.Fatalf("TTL %v (err %v), want about %v", ttl, err, c.TTL)
	}
}

func TestPutInvalidates(t *testing.T) {
	c := newCache(t, false)
	ctx := context.Background()
	if _, _, err := c.Get(ctx, "tea"); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, "tea", "4.00"); err != nil {
		t.Fatal(err)
	}
	if v, hit, err := c.Get(ctx, "tea"); err != nil || hit || v != "4.00" {
		t.Fatalf("after Put: %q hit=%v err=%v, want the new value from the source", v, hit, err)
	}
}

func TestCoalescingStopsAStampede(t *testing.T) {
	c := newCache(t, true)
	c.Source.Delay = 200 * time.Millisecond // long enough that all 100 misses overlap
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, _, err := c.Get(ctx, "tea"); err != nil || v != "3.50" {
				t.Errorf("got %q, %v", v, err)
			}
		}()
	}
	wg.Wait()
	if got := c.Source.Calls(); got != 1 {
		t.Fatalf("100 concurrent misses reached the source %d times, want 1", got)
	}
}

// No Redis needed: a cache that can't be reached must behave like a miss.
func TestBrokenCacheIsAMiss(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	defer rdb.Close()
	c := &Cache{Redis: rdb, Source: NewSource(0), Prefix: "x:", TTL: time.Minute}
	c.Source.Put("tea", "3.50")
	v, hit, err := c.Get(context.Background(), "tea")
	if err != nil || hit || v != "3.50" {
		t.Fatalf("got %q hit=%v err=%v, want the value from the source", v, hit, err)
	}
}
