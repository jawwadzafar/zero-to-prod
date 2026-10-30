# snipdata — the Data Engineering track's code

Python and dbt code for Part 17 of [Zero to Prod](https://jawwadzafar.github.io/zero-to-prod/).
It turns **snip**'s links and click events (snip is the handbook's URL
shortener, in `../snip`) into tables you can answer questions with.

Everything runs **free and on your laptop**. You don't need a running snip:
`generate` writes realistic, seeded data in the same layout.

```bash
cd data
uv sync                                   # create .venv and install everything
uv run snipdata generate                  # 30 days of fake-but-realistic clicks into ./lake
uv run snipdata build                     # dbt build: staging, dim_links, fct_clicks, the daily rollup, and their tests
uv run snipdata query "SELECT * FROM wh.link_clicks_daily ORDER BY clicks DESC LIMIT 5"
uv run pytest && uv run ruff check . && uv run mypy   # tests, lint, types
```

Run it the way production would, one day partition at a time (chapter 17.5):

```bash
uv run snipdata run                                        # yesterday (UTC): sensor, extract, dbt, verify
uv run snipdata backfill --start 2026-10-01 --end 2026-10-28   # every day in a range, oldest first
uv run snipdata runs                                       # each day's latest run, from warehouse/runs.jsonl
```

Each run retries what waiting can fix (a locked warehouse, Postgres
restarting, a partition not there yet) with exponential backoff, never runs
twice at once (`warehouse/snipdata.lock`), and ends by reconciling the
warehouse with the lake. Every step is idempotent: rerunning a day gives the
same rows, and the same fingerprint in the run log. A nightly crontab line:

```text
30 1 * * *  cd /path/to/data && uv run snipdata run >> warehouse/cron.log 2>&1
```

Data quality (chapter 17.7) is checked at three layers:

```bash
uv run snipdata check                      # the clicks contract, file by file (--quarantine moves bad files aside)
uv run snipdata build                      # dbt tests: not_null, unique, relationships, grain, reconciliation,
                                           #   and warnings for redeliveries, missing days and odd volumes
uv run snipdata dbt source freshness       # how old the newest click is (warn after 6 hours, error after 30)
```

`snipdata run` checks the contract before dbt reads anything, and stops on a
violation unless you pass `--quarantine`. Quarantined files go to
`lake/_quarantine/`, each with a `.why.json` note.

With snip's compose stack running (`cd ../snip && docker compose up -d --build --wait`),
copy its real data instead (into a fresh lake folder; `extract` won't mix real rows with generated ones):

```bash
SNIP_DATABASE_URL=postgres://snip:snip@localhost:5432/snip?sslmode=disable \
  uv run snipdata --lake lake-real extract
```

## The lake

```text
lake/
  clicks/dt=YYYY-MM-DD/*.parquet            event_id, slug, clicked_at (UTC, ms): the streaming contract
  links/dt=YYYY-MM-DD/snapshot.parquet      a full copy of snip's links table, one per day
  owners/dt=YYYY-MM-DD/snapshot.parquet     API key owners (never the key hashes)
  audit_events/dt=YYYY-MM-DD/part-*.parquet appended incrementally, by id
```

Click events come from the streaming consumer (chapter 17.6) or from `generate`.
Nothing in `lake/`, `warehouse/` or `dbt/target/` is ever committed.

| Piece | What it does | Chapter |
|---|---|---|
| `snipdata.lake` | The layout, the schemas, and atomic Parquet writes | 17.2 |
| `snipdata.generate` | Seeded, realistic links and clicks | 17.2 |
| `snipdata.extract` | Full snapshots of links and owners; incremental audit events with a watermark | 17.2 |
| `snipdata query` | SQL over the lake with DuckDB | 17.3 |
| `dbt/` | Staging models, `dim_links`, `fct_clicks`, `link_clicks_daily`, and tests | 17.4 |
| `snipdata.orchestrate` | Daily runs, a sensor, retries with backoff, backfills, the run log | 17.5 |
| `snipdata.contract`, `dbt/tests/` | The clicks contract, quarantine, and the data quality tests | 17.7 |
