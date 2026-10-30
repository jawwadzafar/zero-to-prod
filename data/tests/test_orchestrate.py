"""The orchestrator: retries, the sensor, and backfills that give the same answer twice."""

from __future__ import annotations

from datetime import UTC, date, datetime, timedelta
from pathlib import Path

import duckdb
import pyarrow.parquet as pq
import pytest

from snipdata import lake as lk
from snipdata import orchestrate as orc
from snipdata.cli import main
from snipdata.generate import Options, generate

DBT_DIR = Path(__file__).parents[1] / "dbt"
END = date(2026, 10, 29)


@pytest.fixture
def no_sleep(monkeypatch: pytest.MonkeyPatch) -> list[float]:
    slept: list[float] = []
    monkeypatch.setattr(orc, "sleep", slept.append)
    return slept


def config(tmp_path: Path, **kw: object) -> orc.Config:
    return orc.Config(
        lake=tmp_path / "lake",
        warehouse=tmp_path / "wh" / "snip.duckdb",
        dbt_dir=DBT_DIR,
        dsn="postgres://unused",
        **kw,  # type: ignore[arg-type]
    )


def test_backoff_doubles_with_jitter_and_a_cap(tmp_path: Path) -> None:
    cfg = config(tmp_path, backoff=2.0, max_backoff=10.0)
    for attempt, top in [(1, 2.0), (2, 4.0), (3, 8.0), (4, 10.0), (9, 10.0)]:
        delay = orc.backoff_delay(cfg, attempt)
        assert top / 2 <= delay <= top


def test_retryable_failure_is_retried(tmp_path: Path, no_sleep: list[float]) -> None:
    calls = 0

    def flaky(_cfg: orc.Config, _day: date) -> str:
        nonlocal calls
        calls += 1
        if calls < 3:
            raise orc.Retryable("database is locked")
        return "done"

    cfg = config(tmp_path, retries=2)
    assert orc.run_day(cfg, END, [("flaky", flaky)])
    assert [e["status"] for e in orc.read_log(cfg)] == ["retry", "retry", "ok"]
    assert len(no_sleep) == 2


def test_retries_run_out(tmp_path: Path, no_sleep: list[float]) -> None:
    def down(_cfg: orc.Config, _day: date) -> str:
        raise orc.Retryable("connection refused")

    cfg = config(tmp_path, retries=1)
    assert not orc.run_day(cfg, END, [("down", down), ("never", down)])
    assert [(e["step"], e["status"]) for e in orc.read_log(cfg)] == [
        ("down", "retry"),
        ("down", "failed"),
    ]


def test_failed_is_not_retried(tmp_path: Path, no_sleep: list[float]) -> None:
    def bad_data(_cfg: orc.Config, _day: date) -> str:
        raise orc.Failed("not_null_stg_clicks_slug failed")

    cfg = config(tmp_path, retries=5)
    assert not orc.run_day(cfg, END, [("bad", bad_data)])
    assert [e["status"] for e in orc.read_log(cfg)] == ["failed"]
    assert no_sleep == []  # waiting doesn't fix bad data


def test_a_crash_is_logged_and_not_retried(tmp_path: Path, no_sleep: list[float]) -> None:
    def buggy(_cfg: orc.Config, _day: date) -> str:
        return str(1 // 0)

    cfg = config(tmp_path)
    assert not orc.run_day(cfg, END, [("buggy", buggy)])
    assert [(e["status"], e["message"]) for e in orc.read_log(cfg)] == [
        ("failed", "ZeroDivisionError: integer division or modulo by zero")
    ]


def test_sensor_waits_for_the_day_to_end(
    tmp_path: Path, no_sleep: list[float], monkeypatch: pytest.MonkeyPatch
) -> None:
    cfg = config(tmp_path, wait=timedelta(minutes=10), poke=timedelta(minutes=1))
    generate(cfg.lake, Options(days=1, end=END, clicks_per_day=10))
    clock = [datetime(2026, 10, 30, 0, 50, tzinfo=UTC)]  # 50 minutes into the next day
    monkeypatch.setattr(orc, "now", lambda: clock[0])

    def tick(seconds: float) -> None:
        no_sleep.append(seconds)
        clock[0] += timedelta(seconds=seconds)

    monkeypatch.setattr(orc, "sleep", tick)
    assert "1 file(s)" in orc.wait_for_clicks(cfg, END)  # ready at 01:00, after the grace hour
    assert len(no_sleep) == 10


def test_sensor_gives_up_on_a_missing_partition(tmp_path: Path, no_sleep: list[float]) -> None:
    cfg = config(tmp_path)
    with pytest.raises(orc.NotReady, match="no files"):
        orc.wait_for_clicks(cfg, END)


def test_only_one_run_at_a_time(tmp_path: Path) -> None:
    cfg = config(tmp_path)
    with orc.exclusive(cfg), pytest.raises(RuntimeError, match="in progress"), orc.exclusive(cfg):
        pass


def test_days(tmp_path: Path) -> None:
    assert orc.days(date(2026, 10, 30), date(2026, 11, 1)) == [
        date(2026, 10, 30),
        date(2026, 10, 31),
        date(2026, 11, 1),
    ]
    with pytest.raises(ValueError):
        orc.days(END, END - timedelta(days=1))


def fact(cfg: orc.Config) -> list[tuple[object, ...]]:
    with duckdb.connect(str(cfg.warehouse), read_only=True) as con:
        return con.execute(
            "SELECT click_date, count(*), bit_xor(hash(event_id, slug, clicked_at)) "
            "FROM fct_clicks GROUP BY ALL ORDER BY 1"
        ).fetchall()


def test_backfill_then_rerun_gives_identical_results(
    tmp_path: Path, no_sleep: list[float], capsys: pytest.CaptureFixture[str]
) -> None:
    cfg = config(tmp_path)
    generate(cfg.lake, Options(seed=7, days=5, end=END, links=12, clicks_per_day=300))
    # The nightly run for the newest day, then a backfill of the four before it.
    assert orc.run_day(cfg, END)
    assert [r[0] for r in fact(cfg)] == [END]
    assert orc.backfill(cfg, END - timedelta(days=4), END - timedelta(days=1)) == []
    first = fact(cfg)
    assert len(first) == 5

    # Rerun a day in the middle, and backfill everything again: nothing changes.
    assert orc.run_day(cfg, END - timedelta(days=2))
    assert orc.backfill(cfg, END - timedelta(days=4), END) == []
    assert fact(cfg) == first
    assert orc.succeeded_days(cfg) == set(orc.days(END - timedelta(days=4), END))
    capsys.readouterr()
    assert main(["--lake", str(cfg.lake), "runs", "--warehouse", str(cfg.warehouse)]) == 0
    table = capsys.readouterr().out.splitlines()
    assert len(table) == 6 and all(" ok " in line for line in table[1:])

    # A day's file is replaced by a smaller, corrected one and the day is rerun:
    # the day is overwritten whole, so the events that left the lake leave the table.
    day = END - timedelta(days=3)
    path = lk.partition_dir(cfg.lake, "clicks", day) / "generated.parquet"
    lk.write_parquet_atomic(pq.read_table(path).slice(0, 10), path)
    assert orc.run_day(cfg, day)  # verify reconciles the table with the lake
    before = next(r[1] for r in first if r[0] == day)
    after = next(r[1] for r in fact(cfg) if r[0] == day)
    assert isinstance(before, int) and isinstance(after, int) and after < before
