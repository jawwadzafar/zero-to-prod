# snip — the system you build in Zero to Prod

A link shortener, grown chapter by chapter into a production-shaped service.
This folder is the **reference implementation** — the answer key. Every snip
snippet in the handbook is copied from here, and everything here is tested.

> Learning? Write your own version following the chapters first, then compare.
> Start at [0.3 Meet snip](https://jawwadzafar.github.io/zero-to-prod/learn/start/meet-snip).

## Run it (no database needed)

```bash
go run ./cmd/snip        # listens on :8080 and prints a development API key in its log
```

With no `SNIP_DATABASE_URL`, snip stores everything in memory — links *and* API
keys — so it creates a development key at startup (look for `api_key=snip_…`).
Everything is lost when it stops; for real use, run Postgres.

```bash
KEY=snip_...                                   # the key from the log
curl -X POST localhost:8080/api/links -H "Authorization: Bearer $KEY" -d '{"url":"https://go.dev"}'
SNIP_API_KEY=$KEY ./scripts/smoke.sh           # end-to-end check → [smoke] PASS
```

## Run it for real (Postgres + Redis + worker)

```bash
export SNIP_DATABASE_URL="postgres://snip:snip@localhost:5432/snip?sslmode=disable"
export SNIP_REDIS_ADDR="localhost:6379"
go run ./cmd/snip migrate
go run ./cmd/snip keys create me        # copy the snip_… key it prints (and note the owner id)
# later: go run ./cmd/snip keys revoke <owner id>   — the key stops working immediately
go run ./cmd/snip &                     # the API
go run ./cmd/snip-worker &              # counts clicks from the queue

KEY=snip_...
curl -X POST localhost:8080/api/links -H "Authorization: Bearer $KEY" -d '{"url":"https://go.dev"}'
curl -i localhost:8080/<slug>                                   # 302 redirect
curl localhost:8080/api/links/<slug> -H "Authorization: Bearer $KEY"   # clicks
```

## API

| Method & path | Auth | What it does |
|---|---|---|
| `POST /api/links` `{"url": "...", "slug": "optional"}` | key | Create a short link (201) |
| `GET /api/links?limit=20` | key | Your links, newest first |
| `GET /api/links/{slug}` | key | One link with its click count |
| `DELETE /api/links/{slug}` | key | Delete your link (204) |
| `GET /{slug}` | — | Redirect (302) and record a click |
| `GET /healthz` | — | Is the process alive? |
| `GET /readyz` | — | Can it serve? (checks Postgres, Redis) |
| `GET /metrics` | — | Prometheus metrics |

Errors always look like `{"error": {"code": "invalid_url", "message": "..."}}`.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `SNIP_HTTP_ADDR` | `:8080` | Address to listen on |
| `SNIP_BASE_URL` | `http://localhost:8080` | Used to build `short_url` |
| `SNIP_DATABASE_URL` | *(empty → memory)* | Postgres connection string |
| `SNIP_REDIS_ADDR` | *(empty → no cache/queue)* | Redis `host:port` |
| `SNIP_CACHE_TTL` | `1h` | How long redirects stay cached |
| `SNIP_RATE_LIMIT` | `60` | Link creations per key per minute |
| `SNIP_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SNIP_LOG_FORMAT` | `json` | `json` or `text` |
| `SNIP_WORKER_METRICS_ADDR` | `:9091` | The worker's `/metrics` and `/healthz` address |
| `SNIP_DEBUG_ADDR` | (off) | The Go profiler (pprof), e.g. `localhost:6060`. Keep it private (chapter 13.6) |

## Layout

```text
cmd/snip/            API server (+ migrate, keys create)
cmd/snip-worker/     click-counting worker
internal/links/      the domain: Link, validation, Service, interfaces
internal/store/      memory/ and postgres/ (with embedded SQL migrations)
internal/cache/      Redis read-through cache
internal/clicks/     Redis Streams queue: Recorder (API) and Consumer (worker)
internal/ratelimit/  fixed-window limiter (Redis and in-memory)
internal/auth/       API keys: generate, hash, authenticate
internal/httpapi/    routes, handlers, middleware, error mapping
internal/config/     SNIP_* environment variables
internal/metrics/    Prometheus metrics
examples/            tiny standalone programs used by early chapters
```

## Test

```bash
go test ./...                       # unit + HTTP tests, no dependencies needed
SNIP_TEST_DATABASE_URL="postgres://snip:snip@localhost:5432/snip?sslmode=disable" \
SNIP_TEST_REDIS_ADDR=localhost:6379 go test -race ./...   # + Postgres and Redis integration tests
```
