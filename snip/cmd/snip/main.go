// Command snip is the link-shortener API server.
//
//	snip                    start the HTTP server (same as "snip serve")
//	snip migrate            apply database migrations and exit
//	snip keys create NAME   create an API key and print it once
//
// All settings come from SNIP_* environment variables (internal/config).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/cache"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/clicks"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/config"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/httpapi"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/metrics"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/ratelimit"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/memory"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "snip:", err)
		os.Exit(1)
	}
}

// store is what snip needs from its storage: links and API keys.
type store interface {
	links.Store
	auth.KeyStore
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := cfg.Logger()

	// Stop cleanly on Ctrl-C (SIGINT) or when an orchestrator asks (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store
	var pg *postgres.Store
	if cfg.DatabaseURL != "" {
		pg, err = postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pg.Close()
		st = pg
	} else {
		log.Warn("SNIP_DATABASE_URL not set: using in-memory storage — everything is lost on restart")
		st = memory.New()
	}

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "serve" && pg == nil {
		// Keys live in memory too, so a separate "snip keys create" process
		// can't make one for us. Print a development key instead.
		key, _, err := auth.CreateKey(ctx, st, "dev")
		if err != nil {
			return err
		}
		log.Info("in-memory mode: created a development API key (valid until snip stops)", "api_key", key)
	}

	switch cmd {
	case "serve":
		return serve(ctx, cfg, log, st, pg)
	case "migrate":
		if pg == nil {
			return errors.New("migrate needs SNIP_DATABASE_URL")
		}
		applied, err := pg.Migrate(ctx)
		if err != nil {
			return err
		}
		log.Info("migrations applied", "versions", applied)
		return nil
	case "keys":
		if len(args) != 3 || args[1] != "create" {
			return errors.New(`usage: snip keys create NAME`)
		}
		if pg != nil { // make sure the api_keys table exists on a fresh database
			if _, err := pg.Migrate(ctx); err != nil {
				return err
			}
		}
		key, owner, err := auth.CreateKey(ctx, st, args[2])
		if err != nil {
			return err
		}
		fmt.Printf("created API key for %q (owner id %d)\n\n  %s\n\nStore it somewhere safe — it cannot be shown again.\n", owner.Name, owner.ID, key)
		return nil
	default:
		return fmt.Errorf("unknown command %q (want serve, migrate or keys)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger, st store, pg *postgres.Store) error {
	m := metrics.New()
	ready := map[string]httpapi.Pinger{}
	if pg != nil {
		if _, err := pg.Migrate(ctx); err != nil {
			return fmt.Errorf("migrate on startup: %w", err)
		}
		ready["postgres"] = pg
	}

	var (
		linkCache links.Cache
		recorder  links.ClickRecorder
		limiter   ratelimit.Limiter = ratelimit.NewMemory(cfg.RateLimit, time.Minute)
	)
	if cfg.RedisAddr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		defer rdb.Close()
		linkCache = cache.NewRedis(rdb, cfg.CacheTTL)
		recorder = clicks.NewRecorder(rdb)
		limiter = ratelimit.NewRedis(rdb, cfg.RateLimit, time.Minute)
		ready["redis"] = redisPinger{rdb}
	} else {
		log.Info("SNIP_REDIS_ADDR not set: no cache, clicks counted directly")
	}

	api := httpapi.New(httpapi.Options{
		Links:   links.NewService(st, linkCache, recorder),
		Keys:    st,
		Limiter: limiter,
		Logger:  log,
		Metrics: m,
		BaseURL: cfg.BaseURL,
		Ready:   ready,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second, // don't let slow clients hold connections open forever
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("snip listening", "addr", cfg.HTTPAddr, "base_url", cfg.BaseURL)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err // failed to start, e.g. "address already in use"
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting new connections, let in-flight
	// requests finish (up to 10s), then exit.
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("stopped cleanly")
	return nil
}

type redisPinger struct{ rdb *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.rdb.Ping(ctx).Err() }
