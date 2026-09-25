package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newCache(t *testing.T) *Redis {
	t.Helper()
	addr := os.Getenv("SNIP_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("SNIP_TEST_REDIS_ADDR not set")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRedis(rdb, time.Hour)
}

func TestGetSetMiss(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	slug := "cache-test-" + time.Now().Format("150405.000000")
	if _, ok, err := c.GetURL(ctx, slug); ok || err != nil {
		t.Fatalf("empty cache: ok=%v err=%v", ok, err)
	}
	if err := c.SetURL(ctx, slug, "https://go.dev"); err != nil {
		t.Fatal(err)
	}
	if u, ok, err := c.GetURL(ctx, slug); !ok || err != nil || u != "https://go.dev" {
		t.Fatalf("after set: %q ok=%v err=%v", u, ok, err)
	}
}

// The race this guards against: a redirect reads the link from Postgres,
// the owner deletes it (evicting the cache), and then the redirect's cache
// fill puts the deleted link back, where it would redirect for another hour.
func TestLateFillCannotResurrectADeletedLink(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()
	slug := "cache-race-" + time.Now().Format("150405.000000")

	if err := c.Delete(ctx, slug); err != nil { // the owner deletes the link
		t.Fatal(err)
	}
	if err := c.SetURL(ctx, slug, "https://phishing.example"); err != nil { // the late fill
		t.Fatal(err)
	}
	if u, ok, _ := c.GetURL(ctx, slug); ok {
		t.Fatalf("a deleted link came back from the cache: %q", u)
	}
}
