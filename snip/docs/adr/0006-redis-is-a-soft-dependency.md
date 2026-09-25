# 6. Redis is a soft dependency

- **Status:** Accepted
- **Date:** 2026-09-24

## Context

snip uses Redis as a cache for redirects, for the click stream (ADR 5) and for
shared rate-limit counters. It was designed as "never fatal": on a cache error,
fall back to Postgres.

In the game day of chapter 13.5, we froze Redis. Redirects took 15–20 seconds
instead of failing fast (the client retried and waited on long timeouts), and
the readiness check reported not-ready because it checked Redis, so the load
balancer took every copy of snip out of service. A cache had become a hard
dependency without anyone deciding it should.

## Decision

- Readiness fails only for **hard** dependencies (Postgres). Redis is checked
  as a **degradable** one: reported in `/readyz`, never failing it.
- The Redis client fails fast: no retries, a single dial attempt, and short
  timeouts.
- After a cache error, the redirect skips writing the value back to the cache.

## Consequences

- With Redis frozen, redirects take about 0.2 s (served from Postgres) instead
  of hanging; with Redis stopped, about 10 ms. Measured in CI.
- While Redis is down, rate limiting **fails open** (link creation isn't
  limited), clicks made during the outage are **lost** (they never reach the
  stream), and Postgres takes the full read load, so it must have the capacity
  for it. The runbook's `redis-down` section covers the response.
- A general rule for new dependencies: decide explicitly whether each one is
  hard or soft, and test the soft ones failing.
