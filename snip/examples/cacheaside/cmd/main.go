// Command cacheaside shows a Redis cache in front of a slow lookup
// (chapter 6.7): a miss, then hits, invalidation, and a stampede with and
// without request coalescing.
//
//	docker run -d --name cache-lab -p 6379:6379 redis:7-alpine
//	go run ./examples/cacheaside/cmd
//	REDIS_ADDR=localhost:6379 go run ./examples/cacheaside/cmd
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/cacheaside"
)

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "can't reach Redis at %s: %v\n", addr, err)
		os.Exit(1)
	}

	src := cacheaside.NewSource(300 * time.Millisecond) // every lookup at the source costs 300 ms
	src.Put("tea", "3.50")
	c := &cacheaside.Cache{Redis: rdb, Source: src, Prefix: "lab:price:", TTL: 30 * time.Second}
	_ = rdb.Del(ctx, c.Prefix+"tea").Err() // start cold

	fmt.Println("1. Three reads of the same key")
	for range 3 {
		read(ctx, c, "tea")
	}
	ttl, _ := rdb.TTL(ctx, c.Prefix+"tea").Result()
	fmt.Printf("   the copy in Redis expires in %v (30s TTL plus up to 10%% jitter)\n\n", ttl.Round(time.Second))

	fmt.Println("2. The price changes: write the source, then delete the copy")
	if err := c.Put(ctx, "tea", "4.00"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	read(ctx, c, "tea")
	read(ctx, c, "tea")
	fmt.Println()

	for _, coalesce := range []bool{false, true} {
		c.Coalesce = coalesce
		_ = rdb.Del(ctx, c.Prefix+"tea").Err() // the popular key just expired...
		before := src.Calls()
		start := time.Now()
		var wg sync.WaitGroup
		for range 100 { // ...and 100 requests arrive at once
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, _ = c.Get(ctx, "tea")
			}()
		}
		wg.Wait()
		fmt.Printf("3. Stampede, coalescing=%v: 100 concurrent misses sent %d lookup(s) to the source (%v)\n",
			coalesce, src.Calls()-before, time.Since(start).Round(time.Millisecond))
	}
}

func read(ctx context.Context, c *cacheaside.Cache, key string) {
	start := time.Now()
	v, hit, err := c.Get(ctx, key)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	where := "MISS (asked the source)"
	if hit {
		where = "HIT  (from Redis)"
	}
	fmt.Printf("   %s = %s   %-24s %v\n", key, v, where, time.Since(start).Round(time.Microsecond))
}
