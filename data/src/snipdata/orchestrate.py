"""A small orchestrator: one run per day partition, steps in order, retries, a run log.

    uv run snipdata run                        # yesterday (UTC): what cron runs every night
    uv run snipdata run --day 2026-10-28       # one day, again: safe, same result
    uv run snipdata backfill --start 2026-10-01 --end 2026-10-28
    uv run snipdata runs                       # the run log: what ran, when, how it went

A run processes exactly one UTC day, `day`, through these steps:

    wait_for_clicks -> extract -> transform -> verify

- wait_for_clicks is a sensor: the day must be over (plus a grace period for
  late events) and its clicks/dt=<day>/ partition must hold files.
- extract snapshots snip's Postgres, but only for today or yesterday: a
  snapshot shows the database *now*, so it can't be taken for a past day.
- transform runs `dbt build` with start_date = end_date = day, so
  fct_clicks replaces that day's rows and nothing else (chapter 17.4's model).
- verify reconciles: the warehouse must hold exactly the distinct events the
  lake holds for that day, and records a fingerprint of those rows.

Every step is idempotent, so a run can be repeated, and a backfill is many
runs. Why it's a hundred lines of Python and not Airflow or Dagster:
chapter 17.5.
"""

from __future__ import annotations

import fcntl
import json
import random
import time
import uuid
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import UTC, date, datetime, timedelta
from datetime import time as dtime
from pathlib import Path
from typing import Any

import duckdb
import psycopg

from snipdata import extract as ex
from snipdata import lake as lk
from snipdata.transform import run_dbt


class Retryable(Exception):
    """A failure that may go away if we wait: a lock, a network blip, data not there yet."""


class NotReady(Retryable):
    """The sensor's answer: the input partition isn't complete yet."""


class Failed(Exception):
    """A failure that waiting won't fix: a failed test, a mismatch. A person has to look."""


@dataclass(frozen=True)
class Config:
    lake: Path
    warehouse: Path
    dbt_dir: Path
    dsn: str
    grace: timedelta = timedelta(hours=1)  # how late an event may arrive and still count
    wait: timedelta = timedelta(0)  # how long the sensor keeps checking before giving up
    poke: timedelta = timedelta(seconds=30)  # how often it checks
    retries: int = 2  # extra attempts after the first, for Retryable failures
    backoff: float = 2.0  # seconds before the first retry; doubles each time
    max_backoff: float = 60.0

    @property
    def run_log(self) -> Path:
        return self.warehouse.parent / "runs.jsonl"


Step = Callable[[Config, date], str]

# Overridable in tests, so they don't sleep or depend on the clock.
sleep: Callable[[float], None] = time.sleep


def now() -> datetime:
    return datetime.now(UTC)


# ---------------------------------------------------------------- the steps


def wait_for_clicks(cfg: Config, day: date) -> str:
    """Sensor: is the day over (plus grace), and has its partition arrived?"""
    ready_at = datetime.combine(day + timedelta(days=1), dtime(), UTC) + cfg.grace
    deadline = now() + cfg.wait
    while True:
        part = lk.partition_dir(cfg.lake, "clicks", day)
        files = sorted(part.glob("*.parquet"))
        if now() >= ready_at and files:
            return f"{len(files)} file(s) in {part.relative_to(cfg.lake)}"
        reason = (
            f"the day isn't complete until {ready_at:%Y-%m-%d %H:%M} UTC"
            if now() < ready_at
            else f"no files in {part.relative_to(cfg.lake)} yet"
        )
        if now() + cfg.poke > deadline:
            raise NotReady(reason)
        sleep(cfg.poke.total_seconds())


def extract(cfg: Config, day: date) -> str:
    """Snapshot snip's Postgres, when that still means something for `day`."""
    if (cfg.lake / lk.GENERATED_MARKER).exists():
        return "skipped: generated lake, no Postgres to read"
    if day < now().date() - timedelta(days=1):
        return "skipped: a snapshot shows the database now, it can't be taken for a past day"
    try:
        s = ex.extract(cfg.dsn, cfg.lake, day, ex.DEFAULT_SETTLE_SECONDS)
    except psycopg.OperationalError as err:  # can't connect: snip's database may be restarting
        raise Retryable(str(err).strip()) from err
    return f"{s.links} links, {s.owners} owners, {s.audit_events} new audit events"


def transform(cfg: Config, day: date) -> str:
    """dbt build for exactly this day: models first, then their tests."""
    window = json.dumps({"start_date": day.isoformat(), "end_date": day.isoformat()})
    out = run_dbt(cfg.lake, cfg.warehouse, cfg.dbt_dir, ["build", "--vars", window], quiet=True)
    if out.error is not None:  # dbt didn't get to run: a locked warehouse, say
        raise Retryable(out.error)
    summary = ", ".join(f"{n} {s}" for s, n in sorted(out.statuses.items()))
    if not out.success:
        raise Failed(f"dbt build: {summary}; failed: {', '.join(out.failed)}")
    warned = f"; warned: {', '.join(out.warned)}" if out.warned else ""
    return f"dbt build: {summary}{warned}"


def verify(cfg: Config, day: date) -> str:
    """The warehouse holds exactly the lake's distinct events for the day; fingerprint them."""
    lake_glob = (lk.partition_dir(cfg.lake, "clicks", day) / "*.parquet").as_posix()
    with duckdb.connect() as con:
        con.execute("SET TimeZone = 'UTC'")
        con.execute(f"ATTACH '{cfg.warehouse.as_posix()}' AS wh (READ_ONLY)")
        lake_row = con.execute(
            f"SELECT count(DISTINCT event_id) FROM read_parquet('{lake_glob}') "
            "WHERE clicked_at::date = ?",
            [day],
        ).fetchone()
        wh_row = con.execute(
            "SELECT count(*), coalesce(bit_xor(hash(event_id, slug, clicked_at)), 0) "
            "FROM wh.fct_clicks WHERE click_date = ?",
            [day],
        ).fetchone()
    assert lake_row is not None and wh_row is not None
    in_lake, (in_wh, fingerprint) = int(lake_row[0]), wh_row
    if in_wh != in_lake:
        raise Failed(f"fct_clicks has {in_wh} rows for {day}, the lake has {in_lake} events")
    return f"{in_wh} clicks, fingerprint {int(fingerprint):016x}"


STEPS: list[tuple[str, Step]] = [
    ("wait_for_clicks", wait_for_clicks),
    ("extract", extract),
    ("transform", transform),
    ("verify", verify),
]


# ---------------------------------------------------------------- running them


def backoff_delay(cfg: Config, attempt: int) -> float:
    """Exponential backoff with jitter: about 2s, 4s, 8s ... capped, each randomised.

    The jitter spreads out retries, so many failed runs don't all retry at once.
    """
    delay = min(cfg.max_backoff, cfg.backoff * 2 ** (attempt - 1))
    return random.uniform(delay / 2, delay)


def run_day(cfg: Config, day: date, steps: list[tuple[str, Step]] | None = None) -> bool:
    """Run every step for `day`, in order. Stop at the first step that fails for good."""
    run_id = uuid.uuid4().hex[:8]
    for name, step in steps or STEPS:
        attempt = 1
        while True:
            started = time.monotonic()
            try:
                message = step(cfg, day)
                status = "ok"
            except Retryable as err:
                message = str(err)
                status = "retry" if attempt <= cfg.retries else "failed"
            except Failed as err:
                message, status = str(err), "failed"
            except Exception as err:  # a bug, not bad luck: record it, don't retry it
                message, status = f"{type(err).__name__}: {err}", "failed"
            _log(cfg, run_id, day, name, attempt, status, time.monotonic() - started, message)
            if status == "ok":
                break
            if status == "failed":
                return False
            sleep(backoff_delay(cfg, attempt))
            attempt += 1
    return True


def days(start: date, end: date) -> list[date]:
    if end < start:
        raise ValueError(f"--end {end} is before --start {start}")
    return [start + timedelta(days=i) for i in range((end - start).days + 1)]


def backfill(cfg: Config, start: date, end: date, missing_only: bool = False) -> list[date]:
    """Run every day from start to end, oldest first. Return the days that failed.

    The days don't depend on each other, so one failed day doesn't stop the rest.
    """
    done = succeeded_days(cfg) if missing_only else set()
    failed: list[date] = []
    for day in days(start, end):
        if day in done:
            print(f"{day}  already succeeded, skipped")
            continue
        if not run_day(cfg, day):
            failed.append(day)
    return failed


@contextmanager
def exclusive(cfg: Config) -> Iterator[None]:
    """One run at a time: a second cron run that starts while the first is busy exits."""
    cfg.warehouse.parent.mkdir(parents=True, exist_ok=True)
    with (cfg.warehouse.parent / "snipdata.lock").open("w") as f:
        try:
            fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("another run is in progress (snipdata.lock is held)") from None
        yield  # the lock goes when the file closes, even if the process is killed


# ---------------------------------------------------------------- the run log


def _log(
    cfg: Config,
    run_id: str,
    day: date,
    step: str,
    attempt: int,
    status: str,
    seconds: float,
    message: str,
) -> None:
    entry = {
        "at": now().isoformat(timespec="seconds"),
        "run": run_id,
        "day": day.isoformat(),
        "step": step,
        "attempt": attempt,
        "status": status,
        "seconds": round(seconds, 2),
        "message": message,
    }
    print(f"{day}  {step:<15} {status:<6} {seconds:6.1f}s  {message}", flush=True)
    cfg.run_log.parent.mkdir(parents=True, exist_ok=True)
    with cfg.run_log.open("a") as f:
        f.write(json.dumps(entry) + "\n")


def read_log(cfg: Config) -> list[dict[str, Any]]:
    if not cfg.run_log.exists():
        return []
    return [json.loads(line) for line in cfg.run_log.read_text().splitlines() if line]


def latest_runs(cfg: Config) -> dict[date, list[dict[str, Any]]]:
    """Each day's most recent run, as its list of log entries."""
    latest: dict[date, str] = {}
    by_run: dict[str, list[dict[str, Any]]] = {}
    for e in read_log(cfg):
        latest[date.fromisoformat(e["day"])] = e["run"]
        by_run.setdefault(e["run"], []).append(e)
    return {day: by_run[run] for day, run in sorted(latest.items())}


def succeeded_days(cfg: Config) -> set[date]:
    return {
        day
        for day, entries in latest_runs(cfg).items()
        if entries[-1]["step"] == STEPS[-1][0] and entries[-1]["status"] == "ok"
    }
