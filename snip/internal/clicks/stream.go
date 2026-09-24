// Package clicks moves "someone followed a link" events off the redirect path.
// The API appends events to a Redis Stream; the worker reads them in batches,
// adds the counts to Postgres, and acknowledges them.
package clicks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

const (
	// StreamKey is the Redis Stream holding click events.
	StreamKey = "snip:clicks"
	// Group is the consumer group all workers share: each event goes to one worker.
	Group = "workers"
	// maxLen caps the stream so a stopped worker can't fill Redis's memory.
	maxLen = 1_000_000
)

// Recorder appends click events to the stream. It implements links.ClickRecorder.
type Recorder struct {
	rdb *redis.Client
}

// NewRecorder returns a Recorder using rdb.
func NewRecorder(rdb *redis.Client) *Recorder { return &Recorder{rdb: rdb} }

// Record appends one event. XADD is a single fast Redis command, so the
// redirect barely waits for it.
func (r *Recorder) Record(ctx context.Context, slug string) error {
	return r.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamKey,
		MaxLen: maxLen,
		Approx: true,
		Values: map[string]any{"slug": slug, "at": time.Now().UnixMilli()},
	}).Err()
}

// Adder is where counted clicks go (the Postgres store in snip).
type Adder interface {
	AddClicks(ctx context.Context, slug string, n int64) error
}

// Consumer is one worker in the consumer group.
type Consumer struct {
	rdb   *redis.Client
	name  string
	batch int64
	block time.Duration
}

// NewConsumer returns a consumer called name (unique per worker process).
func NewConsumer(rdb *redis.Client, name string) *Consumer {
	return &Consumer{rdb: rdb, name: name, batch: 500, block: 2 * time.Second}
}

// EnsureGroup creates the stream and consumer group if they don't exist yet.
func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, StreamKey, Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// ProcessOnce reads up to one batch of events (waiting up to the block time),
// sums clicks per slug, writes the sums, then acknowledges the events.
// It returns how many events it handled.
//
// Delivery is at-least-once: if the worker crashes after AddClicks but before
// XAck, those events are delivered again and counted twice. For click counts
// that small over-count is an accepted trade-off (chapter 8.2 discusses how to
// make it exactly-once when it matters).
func (c *Consumer) ProcessOnce(ctx context.Context, dst Adder) (int, error) {
	// First re-claim our own events that were delivered but never acked
	// (e.g. we crashed mid-batch), then read new ones (">").
	for _, start := range []string{"0", ">"} {
		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    Group,
			Consumer: c.name,
			Streams:  []string{StreamKey, start},
			Count:    c.batch,
			Block:    c.blockFor(start),
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue // nothing waiting
		}
		if err != nil {
			return 0, err
		}
		var msgs []redis.XMessage
		for _, s := range streams {
			msgs = append(msgs, s.Messages...)
		}
		if len(msgs) == 0 {
			continue
		}
		counts := map[string]int64{}
		ids := make([]string, 0, len(msgs))
		for _, m := range msgs {
			ids = append(ids, m.ID)
			if slug, ok := m.Values["slug"].(string); ok && slug != "" {
				counts[slug]++
			}
		}
		for slug, n := range counts {
			// A link deleted after it was clicked is not an error worth retrying.
			if err := dst.AddClicks(ctx, slug, n); err != nil && !errors.Is(err, links.ErrNotFound) {
				return 0, fmt.Errorf("add %d clicks to %q: %w", n, slug, err)
			}
		}
		if err := c.rdb.XAck(ctx, StreamKey, Group, ids...).Err(); err != nil {
			return 0, err
		}
		return len(msgs), nil
	}
	return 0, nil
}

func (c *Consumer) blockFor(start string) time.Duration {
	if start == "0" {
		return -1 // don't block when checking our pending list
	}
	return c.block
}
