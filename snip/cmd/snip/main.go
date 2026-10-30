// Command snip is the link-shortener API server.
//
//	snip                    start the HTTP server (same as "snip serve")
//	snip migrate            apply database migrations (and create the app's login) and exit
//	snip keys create NAME   create an API key and print it once
//	snip keys revoke ID     revoke the API key of owner ID
//	snip keys rotate ID     replace owner ID's key: the old one stops, links stay
//	snip audit ID           print owner ID's audit trail (newest first) as JSON lines
//
// Each keys command takes --operator NAME: who is running it, recorded in the
// audit trail (or set SNIP_OPERATOR; without either, "unknown" and a warning).
//
// All settings come from SNIP_* environment variables (internal/config).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/audit"
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

// store is what snip needs from its storage: links, API keys, and the audit
// trail of changes to both.
type store interface {
	links.Store
	auth.KeyStore
	audit.Reader
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

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "migrate" {
		applied, logins, err := migrate(ctx, cfg)
		if err != nil {
			return err
		}
		log.Info("migrations applied", "versions", applied)
		if len(logins) > 0 {
			log.Info("database logins ready", "users", logins) // names only, never passwords
		}
		return nil
	}
	if cfg.DatabaseURL != "" && (cmd == "serve" || cmd == "keys") {
		// The schema must be current before snip uses it. This runs before
		// snip connects with SNIP_DATABASE_URL, because with
		// SNIP_MIGRATE_DATABASE_URL set it is what creates that login.
		if _, _, err := migrate(ctx, cfg); err != nil {
			return fmt.Errorf("migrate on startup: %w", err)
		}
	}

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
	case "keys":
		args, operator, err := keyOperator(args, cfg.Operator, os.Stderr)
		if err != nil {
			return err
		}
		if len(args) != 3 || (args[1] != "create" && args[1] != "revoke" && args[1] != "rotate") {
			return errors.New("usage: snip keys create NAME | snip keys revoke OWNER_ID | snip keys rotate OWNER_ID  [--operator NAME]")
		}
		ctx := audit.WithOperator(ctx, operator) // every key event records who ran the command
		if args[1] == "revoke" || args[1] == "rotate" {
			id, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("OWNER_ID must be a number: %w", err)
			}
			if args[1] == "revoke" {
				if err := st.RevokeKey(ctx, id); err != nil {
					return err
				}
				fmt.Printf("revoked API key for owner id %d — it stops working immediately\n", id)
				return nil
			}
			key, err := auth.RotateKey(ctx, st, id)
			if err != nil {
				return err
			}
			fmt.Printf("rotated API key for owner id %d — the old key stops working immediately\n\n  %s\n\nGive the new key to its owner; it cannot be shown again.\n", id, key)
			return nil
		}
		key, owner, err := auth.CreateKey(ctx, st, args[2])
		if err != nil {
			return err
		}
		fmt.Printf("created API key for %q (owner id %d)\n\n  %s\n\nStore it somewhere safe — it cannot be shown again.\n", owner.Name, owner.ID, key)
		return nil
	case "audit":
		if len(args) != 2 {
			return errors.New("usage: snip audit OWNER_ID")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("OWNER_ID must be a number: %w", err)
		}
		events, err := st.AuditLog(ctx, id, 1000)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout) // one JSON object per line, ready for jq
		for _, e := range events {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (want serve, migrate, keys or audit)", cmd)
	}
}

// keyOperator takes "--operator NAME" (or "--operator=NAME") out of a keys
// command's arguments, wherever it is, and returns the other arguments and
// who is running the command: the flag, else fromEnv (SNIP_OPERATOR), else
// audit.UnknownOperator after a warning on stderr. Not the OS user: inside a
// container, where these commands usually run, that is the image's user
// (snip's is "nonroot") whoever runs `docker exec`, and $USER isn't set,
// so it would look like an answer without being one. The name is the
// operator's own claim; chapter 7.4 explains what real systems do instead.
func keyOperator(args []string, fromEnv string, stderr io.Writer) (rest []string, operator string, err error) {
	fromFlag, set := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--operator" || a == "-operator":
			if i+1 == len(args) {
				return nil, "", errors.New("--operator needs a name, e.g. --operator alice")
			}
			i++
			fromFlag, set = args[i], true
		case strings.HasPrefix(a, "--operator=") || strings.HasPrefix(a, "-operator="):
			fromFlag, set = a[strings.Index(a, "=")+1:], true
		default:
			rest = append(rest, a)
		}
	}
	switch {
	case set:
		operator = fromFlag
	case fromEnv != "":
		operator = fromEnv
	default:
		fmt.Fprintf(stderr, "snip: warning: no --operator NAME or SNIP_OPERATOR, so the audit trail records operator %q\n", audit.UnknownOperator)
		return rest, audit.UnknownOperator, nil
	}
	operator = strings.TrimSpace(operator)
	if operator == "" || utf8.RuneCountInString(operator) > 64 || strings.IndexFunc(operator, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return nil, "", errors.New("operator must be 1 to 64 printable characters, e.g. --operator alice")
	}
	return rest, operator, nil
}

// migrate brings the database schema up to date. With
// SNIP_MIGRATE_DATABASE_URL set, it signs in as the schema's owner, applies
// the migrations, then makes sure the logins snip hands out can sign in: the
// app's (SNIP_DATABASE_URL) and, if set, the read-only one
// (SNIP_READONLY_DATABASE_URL). Without it, SNIP_DATABASE_URL's login does
// everything, as in every chapter before 7.4. It returns the versions applied
// and the user names of the logins, never their passwords.
func migrate(ctx context.Context, cfg config.Config) (applied []int, logins []string, err error) {
	url := cfg.MigrateDatabaseURL
	if url == "" {
		if cfg.ReadonlyDatabaseURL != "" {
			return nil, nil, errors.New("SNIP_READONLY_DATABASE_URL needs SNIP_MIGRATE_DATABASE_URL: only the schema's owner creates logins")
		}
		url = cfg.DatabaseURL // one login migrates and serves
	}
	if url == "" {
		return nil, nil, errors.New("migrate needs SNIP_DATABASE_URL (or SNIP_MIGRATE_DATABASE_URL)")
	}
	db, err := postgres.Open(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	if applied, err = db.Migrate(ctx); err != nil || cfg.MigrateDatabaseURL == "" {
		return applied, nil, err
	}
	for _, l := range []struct{ url, role string }{
		{cfg.DatabaseURL, postgres.AppRole},
		{cfg.ReadonlyDatabaseURL, postgres.ReadonlyRole},
	} {
		if l.url == "" {
			continue
		}
		user, err := db.EnsureLogin(ctx, l.url, l.role)
		if err != nil {
			return applied, logins, err
		}
		logins = append(logins, user)
	}
	return applied, logins, nil
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
			// Who may talk to Redis (chapter 7.6): empty password and TLS off
			// = the open, plain Redis of a laptop lab.
			Password:  cfg.RedisPassword,
			TLSConfig: cfg.RedisTLSConfig(),
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
