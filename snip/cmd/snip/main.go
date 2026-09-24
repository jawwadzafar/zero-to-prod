// Command snip is the link-shortener API server.
//
//	snip                    start the HTTP server (same as "snip serve")
//	snip migrate            apply database migrations and exit
//	snip keys create NAME   create an API key and print it once
//	snip keys revoke ID     revoke the API key of owner ID
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
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/cache"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/clicks"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/config"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/debugserver"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/httpapi"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/metrics"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/ratelimit"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/memory"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/postgres"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/tracing"
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
		if len(args) != 3 || (args[1] != "create" && args[1] != "revoke") {
			return errors.New("usage: snip keys create NAME | snip keys revoke OWNER_ID")
		}
		if pg != nil { // make sure the api_keys table exists on a fresh database
			if _, err := pg.Migrate(ctx); err != nil {
				return err
			}
		}
		if args[1] == "revoke" {
			id, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("OWNER_ID must be a number: %w", err)
			}
			if err := st.RevokeKey(ctx, id); err != nil {
				return err
			}
			fmt.Printf("revoked API key for owner id %d — it stops working immediately\n", id)
			return nil
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
	// Tracing (chapter 13.3): on only when OTEL_EXPORTER_OTLP_ENDPOINT is set.
	shutdownTracing, tracingOn, err := tracing.Setup(ctx, "snip-api")
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(flushCtx) // send any spans still in memory
	}()
	if tracingOn {
		log.Info("tracing enabled", "endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}

	m := metrics.New()
	ready := map[string]httpapi.Pinger{}
	soft := map[string]httpapi.Pinger{} // reported by /readyz, never fail it (chapter 13.5)
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
		// Redis is a cache here: when it's down, failing fast and going to
		// Postgres beats waiting. go-redis's defaults (3 command retries,
		// 5 dial attempts, backoff between each) made every redirect take
		// 3–5 s during a Redis outage (chapter 13.5's game day), so: no
		// retries, short timeouts.
		rdb := redis.NewClient(&redis.Options{
			Addr:          cfg.RedisAddr,
			MaxRetries:    -1, // -1 disables command retries; 0 means "the default, 3"
			DialerRetries: 1,  // despite the name, this counts dial *attempts*: 1 = no retry (0 means 5)
			DialTimeout:   250 * time.Millisecond,
			ReadTimeout:   100 * time.Millisecond,
			WriteTimeout:  100 * time.Millisecond,
			PoolTimeout:   200 * time.Millisecond,
		})
		defer rdb.Close()
		linkCache = cache.NewRedis(rdb, cfg.CacheTTL)
		recorder = clicks.NewRecorder(rdb)
		limiter = ratelimit.NewRedis(rdb, cfg.RateLimit, time.Minute)
		soft["redis"] = redisPinger{rdb} // snip degrades without Redis; it doesn't stop
	} else {
		log.Info("SNIP_REDIS_ADDR not set: no cache, clicks counted directly")
	}

	api := httpapi.New(httpapi.Options{
		Links:      links.NewService(st, linkCache, recorder),
		Keys:       st,
		Limiter:    limiter,
		Logger:     log,
		Metrics:    m,
		BaseURL:    cfg.BaseURL,
		Ready:      ready,
		Degradable: soft,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(api.Handler(), "snip"), // a span per request; named by route in observe
		ReadHeaderTimeout: 5 * time.Second,                            // don't let slow clients hold connections open forever
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if cfg.DebugAddr != "" {
		// The profiler on its own private address (chapter 13.6). No write
		// timeout: a CPU profile streams for as many seconds as you ask for.
		dbg := &http.Server{Addr: cfg.DebugAddr, Handler: debugserver.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			log.Warn("pprof debug server enabled; keep it private", "addr", cfg.DebugAddr)
			if err := dbg.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("debug server failed", "err", err)
			}
		}()
		defer dbg.Close()
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
