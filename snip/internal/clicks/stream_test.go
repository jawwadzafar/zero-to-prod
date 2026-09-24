package clicks_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/clicks"
)

type counter struct {
	mu sync.Mutex
	m  map[string]int64
}

func (c *counter) AddClicks(_ context.Context, slug string, n int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[slug] += n
	return nil
}

func TestRecordAndProcess(t *testing.T) {
	addr := os.Getenv("SNIP_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("SNIP_TEST_REDIS_ADDR not set")
	}
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })
	rdb.Del(ctx, clicks.StreamKey) // start clean

	rec := clicks.NewRecorder(rdb)
	for _, slug := range []string{"a", "b", "a", "a"} {
		if err := rec.Record(ctx, slug); err != nil {
			t.Fatal(err)
		}
	}

	c := clicks.NewConsumer(rdb, "test-worker")
	if err := c.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureGroup(ctx); err != nil { // calling twice is fine
		t.Fatal(err)
	}
	if lag, err := c.Lag(ctx); err != nil || lag != 4 {
		t.Fatalf("Lag before processing = %d, %v; want 4 waiting events", lag, err)
	}
	dst := &counter{m: map[string]int64{}}
	n, err := c.ProcessOnce(ctx, dst)
	if err != nil || n != 4 {
		t.Fatalf("ProcessOnce = %d, %v; want 4 events", n, err)
	}
	if dst.m["a"] != 3 || dst.m["b"] != 1 {
		t.Fatalf("counts = %v", dst.m)
	}
	if lag, err := c.Lag(ctx); err != nil || lag != 0 {
		t.Fatalf("Lag after processing = %d, %v; want 0", lag, err)
	}
	pending, err := rdb.XPending(ctx, clicks.StreamKey, clicks.Group).Result()
	if err != nil || pending.Count != 0 {
		t.Fatalf("pending = %+v, %v; want all acknowledged", pending, err)
	}
}
