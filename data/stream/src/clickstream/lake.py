"""Write click events to the lake as Parquet, durably and idempotently.

The layout is the contract shared with the batch pipeline in data/ (dbt reads it):

    <lake>/clicks/dt=YYYY-MM-DD/<anything>.parquet
    columns: event_id (string), slug (string), clicked_at (timestamp, ms, UTC)

Two promises make "acknowledge only after the file is written" safe:

- Durable: a file is written under a temporary name, flushed to disk (fsync),
  then renamed into place. A rename within one directory is atomic, so a reader
  sees either no file or the whole file, never half of one.
- Idempotent: a file's name comes from the first and last event IDs in it. Write
  the same events twice (after a crash, before the acknowledgement) and the
  second file replaces the first instead of sitting next to it.
"""

from __future__ import annotations

import os
from collections import defaultdict
from collections.abc import Iterable
from datetime import date
from pathlib import Path
from typing import Any

import pyarrow as pa
import pyarrow.parquet as pq

from clickstream.events import Click, id_key

_FIELDS: list[pa.Field[Any]] = [
    pa.field("event_id", pa.string(), nullable=False),
    pa.field("slug", pa.string(), nullable=False),
    pa.field("clicked_at", pa.timestamp("ms", tz="UTC"), nullable=False),
]
SCHEMA = pa.schema(_FIELDS)

TMP_SUFFIX = ".tmp"


def clicks_dir(lake: Path) -> Path:
    return lake / "clicks"


def partition_dir(lake: Path, day: date) -> Path:
    return clicks_dir(lake) / f"dt={day.isoformat()}"


def file_name(first_id: str, last_id: str) -> str:
    return f"stream-{first_id}_{last_id}.parquet"


def write_batch(lake: Path, clicks: Iterable[Click]) -> list[Path]:
    """Write clicks into one file per day (by event time) and return the paths.

    Duplicate event IDs in the input are written once.
    """
    by_day: dict[date, dict[str, Click]] = defaultdict(dict)
    for c in clicks:
        by_day[c.clicked_at.date()].setdefault(c.event_id, c)

    paths = []
    for day in sorted(by_day):
        rows = sorted(by_day[day].values(), key=lambda c: id_key(c.event_id))
        table = pa.Table.from_pydict(
            {
                "event_id": [c.event_id for c in rows],
                "slug": [c.slug for c in rows],
                "clicked_at": [c.clicked_at for c in rows],
            },
            schema=SCHEMA,
        )
        path = partition_dir(lake, day) / file_name(rows[0].event_id, rows[-1].event_id)
        write_atomic(table, path)
        paths.append(path)
    return paths


def write_atomic(table: pa.Table, path: Path) -> None:
    """Write table to path so that path is either absent or complete, even after a crash."""
    _mkdir_durable(path.parent)
    # A leading dot and a .tmp suffix: no reader's *.parquet pattern matches it.
    tmp = path.with_name(f".{path.name}.{os.getpid()}{TMP_SUFFIX}")
    with open(tmp, "wb") as f:
        pq.write_table(table, f, compression="zstd")
        f.flush()
        os.fsync(f.fileno())  # the bytes are on disk, not only in the OS's cache
    os.replace(tmp, path)  # atomic: readers see the old state or the new, nothing between
    _fsync_dir(path.parent)  # and the rename itself survives a power cut


def remove_leftovers(lake: Path) -> int:
    """Delete temporary files a crash left behind; return how many."""
    root = clicks_dir(lake)
    if not root.exists():
        return 0
    n = 0
    for tmp in root.glob(f"dt=*/.*{TMP_SUFFIX}"):
        tmp.unlink(missing_ok=True)
        n += 1
    return n


def _mkdir_durable(d: Path) -> None:
    if d.exists():
        return
    _mkdir_durable(d.parent)
    d.mkdir(exist_ok=True)
    _fsync_dir(d.parent)  # a new directory is an entry in its parent: make that durable too


def _fsync_dir(d: Path) -> None:
    fd = os.open(d, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
