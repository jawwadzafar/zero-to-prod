"""The whole batch pipeline on generated data: generate, then dbt build (models and tests).

dbt runs in a child process, as it does when you type `snipdata build`.
"""

from __future__ import annotations

import subprocess
import sys
from datetime import date
from pathlib import Path
from typing import Any

import duckdb
import pyarrow as pa
import pytest

from snipdata import lake as lk
from snipdata.generate import Options, generate

DBT_DIR = Path(__file__).parents[1] / "dbt"
END = date(2026, 10, 29)


def build(lake: Path, warehouse: Path) -> None:
    cmd = ["--lake", str(lake), "build", "--warehouse", str(warehouse), "--dbt-dir", str(DBT_DIR)]
    done = subprocess.run([sys.executable, "-m", "snipdata", *cmd], capture_output=True, text=True)
    assert done.returncode == 0, done.stdout[-3000:]  # every model built and every test passed


def one(warehouse: Path, sql: str) -> tuple[Any, ...] | None:
    with duckdb.connect(str(warehouse), read_only=True) as con:
        return con.execute(sql).fetchone()


@pytest.fixture(scope="module")
def built(tmp_path_factory: pytest.TempPathFactory) -> tuple[Path, Path, int]:
    root = tmp_path_factory.mktemp("pipeline")
    lake, warehouse = root / "lake", root / "wh" / "snip.duckdb"
    s = generate(lake, Options(seed=3, days=14, end=END, links=30, clicks_per_day=600))
    build(lake, warehouse)
    return lake, warehouse, s.clicks


def test_fact_has_each_click_once(built: tuple[Path, Path, int]) -> None:
    _, warehouse, clicks = built
    row = one(warehouse, "SELECT count(*), count(DISTINCT event_id) FROM fct_clicks")
    assert row == (clicks, clicks)  # the redelivered copies are gone


def test_deleted_links_stay_in_the_dimension(built: tuple[Path, Path, int]) -> None:
    _, warehouse, _ = built
    deleted = one(warehouse, "SELECT count(*) FROM dim_links WHERE is_deleted")
    orphans = one(warehouse, "SELECT count(*) FROM fct_clicks ANTI JOIN dim_links USING (slug)")
    assert deleted is not None and deleted[0] >= 1
    assert orphans == (0,)


def test_rebuild_adds_late_events_once(built: tuple[Path, Path, int]) -> None:
    lake, warehouse, clicks = built
    # A late event for yesterday, delivered twice, arrives after the first build.
    late = pa.table(
        {
            "event_id": ["1793236000000-9", "1793236000000-9"],
            "slug": ["golang", "golang"],
            "clicked_at": pa.array([1793236000000, 1793236000000], pa.int64()).cast(lk.UTC_MS),
        },
        schema=lk.CLICKS_SCHEMA,
    )
    lk.write_parquet_atomic(
        late, lk.partition_dir(lake, "clicks", date(2026, 10, 29)) / "late.parquet"
    )
    build(lake, warehouse)  # incremental: reprocesses the last days, replaces by event_id
    build(lake, warehouse)  # and again: a rerun changes nothing
    assert one(warehouse, "SELECT count(*) FROM fct_clicks") == (clicks + 1,)
    assert one(warehouse, "SELECT sum(clicks) FROM link_clicks_daily") == (clicks + 1,)
