"""The clicks contract (snipdata check), and which check catches which kind of broken data."""

from __future__ import annotations

import json
import shutil
from datetime import UTC, date, datetime
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq
import pytest

from snipdata import contract as ct
from snipdata import lake as lk
from snipdata import orchestrate as orc
from snipdata.generate import Options, generate
from snipdata.transform import run_dbt

DBT_DIR = Path(__file__).parents[1] / "dbt"
DAY = date(2026, 10, 28)
NOON = datetime(2026, 10, 28, 12, tzinfo=UTC)


def write(lake: Path, name: str, table: pa.Table, day: date = DAY) -> Path:
    path = lk.partition_dir(lake, "clicks", day) / name
    path.parent.mkdir(parents=True, exist_ok=True)
    pq.write_table(table, path)
    return path


def clicks(ids: list[str], slugs: list[str | None], at: list[datetime]) -> pa.Table:
    return pa.table(
        {
            "event_id": ids,
            "slug": pa.array(slugs, pa.string()),
            "clicked_at": pa.array(at, lk.UTC_MS),
        }
    )


def rules(found: list[ct.Violation]) -> list[tuple[str, str]]:
    return [(x.severity, x.rule) for x in found]


def test_a_good_file_keeps_the_contract(tmp_path: Path) -> None:
    path = write(tmp_path, "ok.parquet", clicks(["1-0", "2-0"], ["a", "b"], [NOON, NOON]))
    assert ct.check_file(path) == []


def test_a_null_slug_breaks_it(tmp_path: Path) -> None:
    path = write(tmp_path, "x.parquet", clicks(["1-0", "2-0"], ["a", None], [NOON, NOON]))
    found = ct.check_file(path)
    assert rules(found) == [("error", "not_null")]
    assert "1 of 2 rows have no slug" in found[0].detail


def test_a_renamed_or_retyped_column_breaks_it_an_extra_one_doesnt(tmp_path: Path) -> None:
    v2 = pa.table({"event_id": ["1-0"], "code": ["a"], "clicked_at": ["2026-10-28T12:00:00Z"]})
    found = ct.check_file(write(tmp_path, "v2.parquet", v2))
    assert rules(found) == [("error", "schema"), ("error", "schema"), ("warn", "schema")]
    assert [x.detail.split(",")[0] for x in found] == [
        "column slug is missing",
        "column clicked_at is string",
        "extra column code: additive",
    ]
    extra = clicks(["1-0"], ["a"], [NOON]).append_column("country", pa.array(["NL"]))
    assert rules(ct.check_file(write(tmp_path, "extra.parquet", extra))) == [("warn", "schema")]


def test_ids_and_partitions_are_checked(tmp_path: Path) -> None:
    yesterday = datetime(2026, 10, 27, 23, 59, tzinfo=UTC)
    path = write(tmp_path, "x.parquet", clicks(["1-0", "abc"], ["a", "b"], [NOON, yesterday]))
    assert rules(ct.check_file(path)) == [("error", "event_id"), ("error", "partition")]


def test_garbage_and_misplaced_files(tmp_path: Path) -> None:
    junk = lk.partition_dir(tmp_path, "clicks", DAY) / "junk.parquet"
    junk.parent.mkdir(parents=True)
    junk.write_text("not parquet")
    assert rules(ct.check_file(junk)) == [("error", "readable")]
    stray = tmp_path / "clicks" / "stray" / "x.parquet"
    stray.parent.mkdir()
    pq.write_table(clicks(["1-0"], ["a"], [NOON]), stray)
    assert rules(ct.check_file(stray)) == [("error", "layout")]


def test_quarantine_moves_bad_files_with_a_note(tmp_path: Path) -> None:
    good = write(tmp_path, "good.parquet", clicks(["1-0"], ["a"], [NOON]))
    bad = write(tmp_path, "bad.parquet", clicks(["2-0"], [None], [NOON]))
    moved = ct.quarantine(tmp_path, ct.check_lake(tmp_path))
    assert moved == [tmp_path / "_quarantine" / "clicks" / "dt=2026-10-28" / "bad.parquet"]
    assert good.exists() and not bad.exists()
    why = json.loads(moved[0].with_name("bad.parquet.why.json").read_text())
    assert why[0]["rule"] == "not_null"
    assert ct.check_lake(tmp_path) == []


def config(tmp_path: Path, quarantine: bool = False) -> orc.Config:
    return orc.Config(
        lake=tmp_path / "lake",
        warehouse=tmp_path / "wh" / "snip.duckdb",
        dbt_dir=DBT_DIR,
        dsn="postgres://unused",
        quarantine=quarantine,
    )


def test_the_run_blocks_or_quarantines(tmp_path: Path) -> None:
    cfg = config(tmp_path)
    write(cfg.lake, "good.parquet", clicks(["1-0"], ["a"], [NOON]))
    write(cfg.lake, "bad.parquet", clicks(["2-0"], [None], [NOON]))
    with pytest.raises(orc.Failed, match=r"bad.parquet: \[not_null\]"):
        orc.check_contract(cfg, DAY)
    message = orc.check_contract(config(tmp_path, quarantine=True), DAY)
    assert message == "quarantined 1 file(s) that break the contract: bad.parquet"
    assert orc.check_contract(cfg, DAY) == "1 file(s) keep the contract"


def build(cfg: orc.Config) -> tuple[bool, list[str], list[str]]:
    out = run_dbt(cfg.lake, cfg.warehouse, cfg.dbt_dir, ["build"], quiet=True)
    assert out.error is None
    return out.success, out.failed, out.warned


def test_which_dbt_check_catches_what(tmp_path: Path) -> None:
    cfg = config(tmp_path)
    end = date(2026, 10, 29)
    generate(cfg.lake, Options(seed=5, days=8, end=end, links=10, clicks_per_day=400))
    assert build(cfg) == (True, [], [])

    # A replayed file: the fact table is still right (duplicates removed), but it warns.
    part = lk.partition_dir(cfg.lake, "clicks", date(2026, 10, 27))
    shutil.copy(part / "generated.parquet", part / "replayed.parquet")
    assert build(cfg) == (True, [], ["clicks_redelivery_rate"])
    (part / "replayed.parquet").unlink()

    # A late partition, on a fresh warehouse: a warning while it's missing ...
    shutil.rmtree(cfg.warehouse.parent)
    late = lk.partition_dir(cfg.lake, "clicks", date(2026, 10, 24))
    shutil.move(late, tmp_path / "late")
    assert build(cfg) == (True, [], ["clicks_partitions_have_no_gaps"])
    # ... and a failure once it lands outside the incremental lookback, until it's loaded.
    shutil.move(tmp_path / "late", late)
    assert build(cfg) == (False, ["fct_clicks_matches_lake"], [])
    assert orc.run_day(cfg, date(2026, 10, 24))
    assert build(cfg) == (True, [], [])

    # A null slug: blocks the build at staging; nothing downstream is rebuilt.
    write(cfg.lake, "bad.parquet", clicks(["1793188800000-0"], [None], [NOON]))
    assert build(cfg) == (False, ["not_null_stg_clicks_slug"], [])
