// Package config reads snip's settings from environment variables, so the
// same built program runs unchanged on a laptop, in CI and in production.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting snip reads at startup.
type Config struct {
	HTTPAddr    string        // SNIP_HTTP_ADDR, default ":8080"
	BaseURL     string        // SNIP_BASE_URL, used to build short links, default "http://localhost:8080"
	DatabaseURL string        // SNIP_DATABASE_URL; empty = in-memory store (data lost on restart)
	RedisAddr   string        // SNIP_REDIS_ADDR (host:port); empty = no cache/queue
	CacheTTL    time.Duration // SNIP_CACHE_TTL, default 1h
	RateLimit   int           // SNIP_RATE_LIMIT: link creations per key per minute, default 60
	LogLevel    slog.Level    // SNIP_LOG_LEVEL: debug|info|warn|error, default info
	LogFormat   string        // SNIP_LOG_FORMAT: json|text, default json

	WorkerMetricsAddr string // SNIP_WORKER_METRICS_ADDR: the worker's /metrics and /healthz, default ":9091"
}

// Load reads the environment. It returns an error for malformed values
// rather than silently falling back — a typo in production config should
// stop the program at startup, not surprise you at 3 a.m.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:    env("SNIP_HTTP_ADDR", ":8080"),
		BaseURL:     strings.TrimRight(env("SNIP_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL: os.Getenv("SNIP_DATABASE_URL"),
		RedisAddr:   os.Getenv("SNIP_REDIS_ADDR"),
		LogFormat:   env("SNIP_LOG_FORMAT", "json"),

		WorkerMetricsAddr: env("SNIP_WORKER_METRICS_ADDR", ":9091"),
	}
	var err error
	if c.CacheTTL, err = time.ParseDuration(env("SNIP_CACHE_TTL", "1h")); err != nil {
		return c, fmt.Errorf("SNIP_CACHE_TTL: %w", err)
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

// Logger builds the structured logger described by the config.
func (c Config) Logger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	if c.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
