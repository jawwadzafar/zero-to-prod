"""The lake: Parquet files in folders, one folder per table and per day.

    lake/
      clicks/dt=2026-10-29/<anything>.parquet       one row per click (the streaming contract)
      links/dt=2026-10-29/snapshot.parquet          a full copy of snip's links table that day
      owners/dt=2026-10-29/snapshot.parquet         API key owners (never the key hashes)
      audit_events/dt=2026-10-29/part-<ids>.parquet appended incrementally, by id

`dt=` folders are Hive-style partitions: DuckDB, Spark and Trino read the
folder name as a column and skip folders a query filters out (chapter 17.3).
Every write goes to a temporary file first and is then renamed into place,
so a reader or a crash never sees half a file (chapter 17.2).
"""

from __future__ import annotations

import os
from datetime import date
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

UTC_MS = pa.timestamp("ms", tz="UTC")
UTC_US = pa.timestamp("us", tz="UTC")

# The contract shared with the streaming consumer (chapter 17.6): column names,
# types and layout. Changing it breaks the other side, so it lives in one place.
CLICKS_SCHEMA = pa.schema(
    [
        # the Redis Stream entry ID, e.g. 1761800000000-0
        pa.field("event_id", pa.string(), nullable=False),
        pa.field("slug", pa.string(), nullable=False),
        pa.field("clicked_at", UTC_MS, nullable=False),
    ]
)

LINKS_SCHEMA = pa.schema(
    [
        pa.field("slug", pa.string(), nullable=False),
        pa.field("url", pa.string(), nullable=False),
        pa.field("owner_id", pa.int64(), nullable=False),
        # snip's own click counter, kept for reconciliation
        pa.field("clicks", pa.int64(), nullable=False),
        pa.field("created_at", UTC_US, nullable=False),
        pa.field("extracted_at", UTC_US, nullable=False),
    ]
)

OWNERS_SCHEMA = pa.schema(
    [
        pa.field("owner_id", pa.int64(), nullable=False),
        pa.field("name", pa.string(), nullable=False),
        pa.field("created_at", UTC_US, nullable=False),
        pa.field("revoked_at", UTC_US),
        pa.field("extracted_at", UTC_US, nullable=False),
    ]
)

AUDIT_SCHEMA = pa.schema(
    [
        pa.field("id", pa.int64(), nullable=False),
        pa.field("at", UTC_US, nullable=False),
        pa.field("owner_id", pa.int64(), nullable=False),
        pa.field("action", pa.string(), nullable=False),
        pa.field("slug", pa.string()),
        pa.field("request_id", pa.string()),
        pa.field("detail", pa.string()),  # the JSON text, e.g. {"url": "https://go.dev"}
    ]
)

TABLES = ("clicks", "links", "owners", "audit_events")

# Written by `snipdata generate`, so `extract` can refuse to mix real rows into fake ones.
GENERATED_MARKER = "_generated.json"


def default_lake() -> Path:
    """The lake's root folder: $SNIPDATA_LAKE, or ./lake."""
    return Path(os.environ.get("SNIPDATA_LAKE", "lake"))


def partition_dir(lake: Path, table: str, day: date) -> Path:
    """lake/<table>/dt=YYYY-MM-DD"""
    return lake / table / f"dt={day.isoformat()}"


def write_parquet_atomic(table: pa.Table, path: Path) -> None:
    """Write table to path so that readers see the old file or the new one, never a partial one.

    Writing the same rows to the same path twice leaves the same file: the
    write is idempotent, which is what makes reruns safe.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(f".{path.name}.{os.getpid()}.tmp")
    pq.write_table(table, tmp, compression="zstd")
    os.replace(tmp, path)  # atomic on the same filesystem


def glob(lake: Path, table: str) -> str:
    """The glob that matches every file of a table, for read_parquet()."""
    return (lake / table / "*" / "*.parquet").as_posix()


def has_files(lake: Path, table: str) -> bool:
    return any((lake / table).glob("*/*.parquet"))
