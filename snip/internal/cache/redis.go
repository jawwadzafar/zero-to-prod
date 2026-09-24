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

func (c *Redis) GetURL(ctx context.Context, slug string) (string, bool, error) {
	u, err := c.rdb.Get(ctx, key(slug)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil // a miss, not an error
	}
	if err != nil {
		return "", false, err
	}
	return u, true, nil
}

func (c *Redis) SetURL(ctx context.Context, slug, url string) error {
	return c.rdb.Set(ctx, key(slug), url, c.ttl).Err()
}

func (c *Redis) Delete(ctx context.Context, slug string) error {
	return c.rdb.Del(ctx, key(slug)).Err()
}
