// Package cache keeps recently used slug→URL pairs in Redis so most redirects
// never touch Postgres.
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a links.Cache backed by Redis keys "snip:url:<slug>".
type Redis struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRedis returns a cache whose entries expire after ttl, so memory stays
// bounded and stale entries eventually disappear on their own.
func NewRedis(rdb *redis.Client, ttl time.Duration) *Redis {
	return &Redis{rdb: rdb, ttl: ttl}
}

func key(slug string) string { return "snip:url:" + slug }

// tombstone marks a slug as just deleted. It reads as a miss, and because
// SetURL only fills empty keys, it stops a redirect that looked the link up
// just before the delete from putting it back in the cache just after.
const (
	tombstone    = ""
	tombstoneTTL = time.Minute // far longer than any in-flight lookup
)

func (c *Redis) GetURL(ctx context.Context, slug string) (string, bool, error) {
	u, err := c.rdb.Get(ctx, key(slug)).Result()
	if errors.Is(err, redis.Nil) || (err == nil && u == tombstone) {
		return "", false, nil // a miss, not an error
	}
	if err != nil {
		return "", false, err
	}
	return u, true, nil
}

// SetURL fills the cache only if the key is empty (SET NX): links never
// change, so there's nothing to overwrite except a tombstone, which must win.
func (c *Redis) SetURL(ctx context.Context, slug, url string) error {
	return c.rdb.SetNX(ctx, key(slug), url, c.ttl).Err()
}

// Delete replaces the entry with a short-lived tombstone rather than just
// removing it (see tombstone above).
func (c *Redis) Delete(ctx context.Context, slug string) error {
	return c.rdb.Set(ctx, key(slug), tombstone, tombstoneTTL).Err()
}
