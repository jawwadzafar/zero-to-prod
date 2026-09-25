// Command snip-worker reads click events from the Redis Stream and adds them
// to link counts in Postgres. Run as many copies as you like: the consumer
// group gives each event to exactly one of them.
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	// A small HTTP server so Prometheus can scrape the worker and Kubernetes
	// can probe it (chapter 13.2).
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})
	srv := &http.Server{Addr: cfg.WorkerMetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failed", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Info("worker started", "stream", clicks.StreamKey, "group", clicks.Group, "metrics_addr", cfg.WorkerMetricsAddr)

	backoff := time.Second
	for ctx.Err() == nil {
		n, err := consumer.ProcessOnce(ctx, pg)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			// Jitter (chapter 8.1): wait a random time between half and all of the
			// backoff, so workers that failed together don't retry in lockstep.
			wait := backoff/2 + rand.N(backoff/2+1)
			log.Error("processing clicks failed; retrying", "err", err, "retry_in", wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, 30*time.Second) // exponential backoff (chapter 8.1)
			continue
		}
		backoff = time.Second
		if lag, err := consumer.Lag(ctx); err == nil {
			m.WorkerLag.Set(float64(lag))
		}
		if n > 0 {
			m.ClicksProcessed.Add(float64(n))
			log.Debug("processed clicks", "events", n)
		}
	}
	log.Info("worker stopped")
	return nil
}
