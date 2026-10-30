# clickstream — snip's click events, streamed into the lake

The streaming half of the Data Engineering track (chapter 17.6 of
[Zero to Prod](https://jawwadzafar.github.io/zero-to-prod/)).

snip's API appends one event per redirect to the Redis stream `snip:clicks`
(`slug`, and `at` in unix milliseconds; see `snip/internal/clicks/stream.go`).
snip's own workers count those events in Postgres, in the consumer group
`workers`. This program is a second consumer group, `analytics`, that sees
every event too and lands it in the lake as Parquet:

```text
<LAKE_DIR>/clicks/dt=YYYY-MM-DD/stream-<first ID>_<last ID>.parquet
  event_id    string                 the stream entry ID: the deduplication key
  slug        string
  clicked_at  timestamp[ms, UTC]     event time, from the event's `at`
```

- **No loss on a crash.** A batch is acknowledged (`XACK`) only after its file
  is written to a temporary name, fsynced and atomically renamed into place.
  Crash before the acknowledgement and the batch is delivered again.
- **No duplicates from redelivery.** A file is named by its first and last
  event IDs, so a redelivered batch rewrites the same file.
- **Restarts and dead consumers.** It reads its own pending events first, then
  claims other analytics consumers' events idle for over a minute (`XAUTOCLAIM`).
- **Honest gaps.** snip caps the stream (`MAXLEN ~ 1000000`). If this consumer
  falls so far behind that unread events are trimmed, it logs `DATA GAP` with
  the count and appends a record to `<LAKE_DIR>/_gaps/clicks.jsonl`.

## Run it

With snip's compose stack (from `snip/`):

```bash
mkdir -p ../data/lake                                  # owned by you, so the container can write as you
docker compose --profile analytics up -d --build       # the stack plus the analytics consumer
docker compose logs -f analytics
```

On its own, against any Redis:

```bash
cd data/stream
uv sync
SNIP_REDIS_ADDR=localhost:6379 SNIP_REDIS_PASSWORD=... LAKE_DIR=../lake uv run python -m clickstream
```

| Variable | Default | Meaning |
|---|---|---|
| `SNIP_REDIS_ADDR` | `localhost:6379` | Redis `host:port`, as snip uses it |
| `SNIP_REDIS_PASSWORD` | empty | Redis password |
| `LAKE_DIR` | `../lake` | the lake's root directory |
| `CLICKSTREAM_CONSUMER` | hostname | this consumer's name in the group |
| `CLICKSTREAM_BATCH_SIZE` | `1000` | events per file, at most |
| `CLICKSTREAM_FLUSH_SECONDS` | `10` | write a smaller batch once its oldest event waited this long |
| `CLICKSTREAM_CLAIM_IDLE_SECONDS` | `60` | claim another consumer's pending events after this long |
| `CLICKSTREAM_CRASH_AFTER_WRITE` | off | lab only: `1` exits after writing a file, before `XACK` |

## Develop

```bash
uv sync
uv run ruff check . && uv run ruff format --check . && uv run mypy
docker run -d --name stream-test -p 6379:6379 redis:7-alpine
TEST_REDIS_URL=redis://localhost:6379/15 uv run pytest     # the Redis tests skip without it
```
