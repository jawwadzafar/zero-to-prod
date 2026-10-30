"""Copy snip's Postgres tables into the lake, safely and repeatably.

Two patterns, chosen per table (chapter 17.2):

- links and owners are small and change in place (a link's click counter
  goes up, a key gets revoked), so each run takes a **full snapshot** into
  lake/<table>/dt=<today>/snapshot.parquet. Running twice on the same day
  replaces that day's file; yesterday's file stays, which keeps history.
- audit_events only ever grows (snip's database refuses UPDATE and DELETE on
  it), so each run copies **only the new rows**. The **watermark** (the
  highest id already copied) is read from the lake itself, not from a separate
  state file: the Parquet file is the only thing a run commits, so a run that
  dies half-way leaves nothing to clean up and the next run starts from the
  right place.

Neither ever reads api_keys.key_hash: the lake holds no secrets.
"""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from datetime import UTC, date, datetime
from pathlib import Path
from typing import Any

import duckdb
import psycopg
import pyarrow as pa

from snipdata import lake as lk

# Rows whose transaction started less than this long ago are left for the next
# run. A sequence hands out ids when rows are inserted, not when they commit,
# so a slow transaction can commit id 41 after id 42 is already visible. If we
# copied 42 and moved the watermark past 41, row 41 would be skipped forever.
DEFAULT_SETTLE_SECONDS = 5

LINKS_SQL = """
SELECT slug, url, owner_id, clicks, created_at, now() AS extracted_at
FROM links
ORDER BY slug
"""

# Every column of api_keys except key_hash.
OWNERS_SQL = """
SELECT id AS owner_id, name, created_at, revoked_at, now() AS extracted_at
FROM api_keys
ORDER BY id
"""

AUDIT_SQL = """
SELECT id, at, owner_id, action, slug, request_id, detail::text AS detail
FROM audit_events
WHERE id > %(after)s
  AND at < now() - make_interval(secs => %(settle)s)
ORDER BY id
LIMIT %(batch)s
"""


@dataclass(frozen=True)
class Summary:
    snapshot_day: date
    links: int
    owners: int
    audit_events: int  # new rows copied this run
    watermark_before: int
    watermark_after: int


def audit_watermark(lake: Path) -> int:
    """The highest audit_events id already in the lake (0 if none)."""
    if not lk.has_files(lake, "audit_events"):
        return 0
    with duckdb.connect() as con:
        row = con.execute(
            f"SELECT max(id) FROM read_parquet('{lk.glob(lake, 'audit_events')}')"
        ).fetchone()
    return int(row[0]) if row and row[0] is not None else 0


def _table(rows: list[tuple[Any, ...]], schema: pa.Schema) -> pa.Table:
    columns: list[tuple[Any, ...]] = (
        list(zip(*rows, strict=True)) if rows else [() for _ in schema.names]
    )
    return pa.table(
        {
            name: pa.array(col, type=schema.field(name).type)
            for name, col in zip(schema.names, columns, strict=True)
        },
        schema=schema,
    )


def snapshot(conn: psycopg.Connection[Any], lake: Path, day: date) -> tuple[int, int]:
    """Write today's full copy of links and owners. Both come from one consistent view."""
    # REPEATABLE READ: both queries see the database as it was at the first one,
    # so no link points at an owner the owners file doesn't have.
    with conn.transaction():
        conn.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
        links = conn.execute(LINKS_SQL).fetchall()
        owners = conn.execute(OWNERS_SQL).fetchall()
    lk.write_parquet_atomic(
        _table(links, lk.LINKS_SCHEMA), lk.partition_dir(lake, "links", day) / "snapshot.parquet"
    )
    lk.write_parquet_atomic(
        _table(owners, lk.OWNERS_SCHEMA), lk.partition_dir(lake, "owners", day) / "snapshot.parquet"
    )
    return len(links), len(owners)


def append_audit(
    conn: psycopg.Connection[Any],
    lake: Path,
    settle_seconds: float = DEFAULT_SETTLE_SECONDS,
    batch: int = 10_000,
) -> tuple[int, int, int]:
    """Copy audit events newer than the lake's watermark. Returns (copied, before, after)."""
    before = after = audit_watermark(lake)
    copied = 0
    while True:
        rows = conn.execute(
            AUDIT_SQL, {"after": after, "settle": settle_seconds, "batch": batch}
        ).fetchall()
        if not rows:
            break
        # One file per day the events happened on, named by the id range it holds:
        # a rerun that fetches the same rows writes the same file name.
        by_day: dict[date, list[tuple[Any, ...]]] = defaultdict(list)
        for r in rows:
            by_day[r[1].astimezone(UTC).date()].append(r)
        for day in sorted(by_day):  # oldest first, so the watermark only moves forward
            day_rows = by_day[day]
            name = f"part-{day_rows[0][0]:012d}-{day_rows[-1][0]:012d}.parquet"
            lk.write_parquet_atomic(
                _table(day_rows, lk.AUDIT_SCHEMA),
                lk.partition_dir(lake, "audit_events", day) / name,
            )
        copied += len(rows)
        after = int(rows[-1][0])
        if len(rows) < batch:
            break
    return copied, before, after


def extract(
    dsn: str,
    lake: Path,
    day: date | None = None,
    settle_seconds: float = DEFAULT_SETTLE_SECONDS,
) -> Summary:
    """Snapshot links and owners, and append new audit events, from snip's database at dsn."""
    if (lake / lk.GENERATED_MARKER).exists():
        raise RuntimeError(
            f"{lake} holds generated data ({lk.GENERATED_MARKER}); "
            "extract into another folder with --lake, or delete it first"
        )
    day = day or datetime.now(UTC).date()
    with psycopg.connect(dsn, autocommit=True, application_name="snipdata-extract") as conn:
        links, owners = snapshot(conn, lake, day)
        copied, before, after = append_audit(conn, lake, settle_seconds)
    return Summary(day, links, owners, copied, before, after)
