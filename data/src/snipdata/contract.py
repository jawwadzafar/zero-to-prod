"""The clicks contract, checked at the door: every file in the lake, before dbt reads it.

    uv run snipdata check                       # every clicks file; exit 1 on a violation
    uv run snipdata check --day 2026-10-29      # one day's partition
    uv run snipdata check --quarantine          # move bad files aside instead, and carry on

The contract (lake.CLICKS_SCHEMA, shared with the streaming consumer) says
what a producer promises a consumer (chapter 17.7):

- the columns event_id (string), slug (string), clicked_at (timestamp, ms, UTC),
  with those names and exactly those types;
- none of them is ever null;
- event_id looks like a Redis Stream entry ID, <milliseconds>-<sequence>;
- a click sits in the dt= folder of the UTC day it happened (event time).

A new extra column is an additive change: allowed, and reported as a warning.
Anything else is a breaking change for the consumer, and an error.
"""

from __future__ import annotations

import json
import re
import shutil
from dataclasses import asdict, dataclass
from datetime import date
from pathlib import Path

import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.parquet as pq

from snipdata import lake as lk

EVENT_ID = re.compile(r"^\d+-\d+$")
QUARANTINE = "_quarantine"


@dataclass(frozen=True)
class Violation:
    path: Path
    severity: str  # "error" breaks the contract; "warn" is allowed but worth knowing
    rule: str
    detail: str

    def __str__(self) -> str:
        return f"{self.severity.upper():<5} {self.path}  [{self.rule}] {self.detail}"


def partition_day(path: Path) -> date | None:
    name = path.parent.name
    try:
        return date.fromisoformat(name.removeprefix("dt=")) if name.startswith("dt=") else None
    except ValueError:
        return None


def check_file(path: Path) -> list[Violation]:
    """Every way one clicks file breaks (or stretches) the contract."""

    def v(severity: str, rule: str, detail: str) -> Violation:
        return Violation(path, severity, rule, detail)

    day = partition_day(path)
    if day is None:
        return [v("error", "layout", "not inside a clicks/dt=YYYY-MM-DD/ folder")]
    try:
        schema = pq.read_schema(path)
    except (OSError, pa.ArrowInvalid) as err:
        return [v("error", "readable", f"not a Parquet file: {err}")]

    found: list[Violation] = []
    for field in lk.CLICKS_SCHEMA:
        if field.name not in schema.names:
            found.append(v("error", "schema", f"column {field.name} is missing"))
        elif not schema.field(field.name).type.equals(field.type):
            found.append(
                v(
                    "error",
                    "schema",
                    f"column {field.name} is {schema.field(field.name).type}, "
                    f"the contract says {field.type}",
                )
            )
    for name in schema.names:
        if name not in lk.CLICKS_SCHEMA.names:
            found.append(v("warn", "schema", f"extra column {name}: additive, consumers ignore it"))
    if any(f.severity == "error" for f in found):
        return found  # the content checks below need the right columns

    t = pq.read_table(path, columns=lk.CLICKS_SCHEMA.names)
    for name in lk.CLICKS_SCHEMA.names:
        nulls = t.column(name).null_count
        if nulls:
            found.append(v("error", "not_null", f"{nulls} of {t.num_rows} rows have no {name}"))
    ids = [i for i in t.column("event_id").to_pylist() if i is not None]
    bad_ids = [i for i in ids if not EVENT_ID.match(i)]
    if bad_ids:
        found.append(
            v("error", "event_id", f"{len(bad_ids)} ids aren't <ms>-<seq>, e.g. {bad_ids[0]!r}")
        )
    days = pc.cast(pc.drop_null(t.column("clicked_at")), pa.date32()).to_pylist()
    outside = [d for d in days if d != day]
    if outside:
        found.append(
            v(
                "error",
                "partition",
                f"{len(outside)} clicks happened on another day than dt={day}, e.g. {outside[0]}",
            )
        )
    return found


def files(lake: Path, day: date | None = None) -> list[Path]:
    root = lk.partition_dir(lake, "clicks", day) if day else lake / "clicks"
    pattern = "*.parquet" if day else "*/*.parquet"
    return sorted(root.glob(pattern))


def check_lake(lake: Path, day: date | None = None) -> list[Violation]:
    return [x for path in files(lake, day) for x in check_file(path)]


def quarantine(lake: Path, violations: list[Violation]) -> list[Path]:
    """Move every file with an error to lake/_quarantine/, with a note saying why.

    Nothing is deleted: a person can read the note, fix the producer, and move
    the file back (or replace it) and rerun the day.
    """
    moved: list[Path] = []
    for path in sorted({x.path for x in violations if x.severity == "error"}):
        dest = lake / QUARANTINE / path.relative_to(lake)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(path, dest)
        reasons = [
            {k: str(val) for k, val in asdict(x).items()} for x in violations if x.path == path
        ]
        dest.with_name(dest.name + ".why.json").write_text(json.dumps(reasons, indent=2) + "\n")
        moved.append(dest)
    return moved
