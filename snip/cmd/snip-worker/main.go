// Command snip-worker reads click events from the Redis Stream and adds them
// to link counts in Postgres. Run as many copies as you like: the consumer
// group gives each event to exactly one of them.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/clicks"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/config"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/metrics"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "snip-worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := cfg.Logger()
	if cfg.DatabaseURL == "" || cfg.RedisAddr == "" {
		return errors.New("snip-worker needs both SNIP_DATABASE_URL and SNIP_REDIS_ADDR")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pg.Close()
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	host, _ := os.Hostname()
	consumer := clicks.NewConsumer(rdb, fmt.Sprintf("%s-%d", host, os.Getpid()))
	if err := consumer.EnsureGroup(ctx); err != nil {
		return err
	}
	m := metrics.New()
	log.Info("worker started", "stream", clicks.StreamKey, "group", clicks.Group)

	backoff := time.Second
	for ctx.Err() == nil {
		n, err := consumer.ProcessOnce(ctx, pg)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Error("processing clicks failed; retrying", "err", err, "retry_in", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, 30*time.Second) // exponential backoff (chapter 8.4)
			continue
		}
		backoff = time.Second
		if n > 0 {
			m.ClicksProcessed.Add(float64(n))
			log.Debug("processed clicks", "events", n)
		}
	}
	log.Info("worker stopped")
	return nil
}
