// Package config reads snip's settings from environment variables, so the
// same built program runs unchanged on a laptop, in CI and in production.
package config

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting snip reads at startup.
type Config struct {
	HTTPAddr      string        // SNIP_HTTP_ADDR, default ":8080"
	BaseURL       string        // SNIP_BASE_URL, used to build short links, default "http://localhost:8080"
	DatabaseURL   string        // SNIP_DATABASE_URL; empty = in-memory store (data lost on restart)
	RedisAddr     string        // SNIP_REDIS_ADDR (host:port); empty = no cache/queue
	RedisPassword string        // SNIP_REDIS_PASSWORD; empty = connect without a password (never logged)
	RedisTLS      bool          // SNIP_REDIS_TLS: "true" = encrypt and verify the server's certificate; default false
	CacheTTL      time.Duration // SNIP_CACHE_TTL, default 1h
	RateLimit     int           // SNIP_RATE_LIMIT: link creations per key per minute, default 60
	LogLevel      slog.Level    // SNIP_LOG_LEVEL: debug|info|warn|error, default info
	LogFormat     string        // SNIP_LOG_FORMAT: json|text, default json
	DebugAddr     string        // SNIP_DEBUG_ADDR: pprof profiler, e.g. "localhost:6060"; empty = off (chapter 13.6)

	WorkerMetricsAddr string // SNIP_WORKER_METRICS_ADDR: the worker's /metrics and /healthz, default ":9091"

	// Least privilege in the database (chapter 7.4). Both optional: without
	// them, SNIP_DATABASE_URL's login both migrates the schema and serves.
	MigrateDatabaseURL  string // SNIP_MIGRATE_DATABASE_URL: the schema owner's login, used only to migrate; SNIP_DATABASE_URL is then the app's own login
	ReadonlyDatabaseURL string // SNIP_READONLY_DATABASE_URL: a read-only login for analytics, created by "snip migrate"
}

// Load reads the environment. It returns an error for malformed values
// rather than silently falling back — a typo in production config should
// stop the program at startup, not surprise you at 3 a.m.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:      env("SNIP_HTTP_ADDR", ":8080"),
		BaseURL:       strings.TrimRight(env("SNIP_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL:   os.Getenv("SNIP_DATABASE_URL"),
		RedisAddr:     os.Getenv("SNIP_REDIS_ADDR"),
		RedisPassword: os.Getenv("SNIP_REDIS_PASSWORD"),
		LogFormat:     env("SNIP_LOG_FORMAT", "json"),
		DebugAddr:     os.Getenv("SNIP_DEBUG_ADDR"),

		WorkerMetricsAddr: env("SNIP_WORKER_METRICS_ADDR", ":9091"),

		MigrateDatabaseURL:  os.Getenv("SNIP_MIGRATE_DATABASE_URL"),
		ReadonlyDatabaseURL: os.Getenv("SNIP_READONLY_DATABASE_URL"),
	}
	var err error
	if c.CacheTTL, err = time.ParseDuration(env("SNIP_CACHE_TTL", "1h")); err != nil {
		return c, fmt.Errorf("SNIP_CACHE_TTL: %w", err)
	}
	if c.RedisTLS, err = strconv.ParseBool(env("SNIP_REDIS_TLS", "false")); err != nil {
		return c, fmt.Errorf("SNIP_REDIS_TLS: must be true or false")
	}
	if c.RateLimit, err = strconv.Atoi(env("SNIP_RATE_LIMIT", "60")); err != nil || c.RateLimit < 1 {
		return c, fmt.Errorf("SNIP_RATE_LIMIT: must be a positive integer")
	}
	if err := c.LogLevel.UnmarshalText([]byte(env("SNIP_LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("SNIP_LOG_LEVEL: %w", err)
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return c, fmt.Errorf("SNIP_LOG_FORMAT: must be json or text")
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// RedisTLSConfig returns the TLS settings for connecting to Redis: nil (a
// plain connection) unless SNIP_REDIS_TLS is on. With TLS, Go checks the
// server's certificate against the host name in SNIP_REDIS_ADDR, as a browser
// does for a website (chapter 7.1). There is deliberately no switch to skip
// that check.
func (c Config) RedisTLSConfig() *tls.Config {
	if !c.RedisTLS {
		return nil
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

// Logger builds the structured logger described by the config.
func (c Config) Logger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	if c.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
