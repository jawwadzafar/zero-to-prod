"""snipdata: one command for each step of the pipeline.

    uv run snipdata generate             # fake but realistic data, no snip needed
    uv run snipdata extract              # or: copy snip's Postgres into the lake
    uv run snipdata build                # dbt build: models + tests, into the warehouse
    uv run snipdata dbt docs generate    # any other dbt command
    uv run snipdata query "SELECT ..."   # SQL over the lake (and the warehouse, as wh.*)
    uv run snipdata run                  # the orchestrated daily run, for yesterday (UTC)
    uv run snipdata backfill --start 2026-10-01 --end 2026-10-28
    uv run snipdata runs                 # the run log: each day's latest run

Run it from the data/ folder. Paths come from flags or environment variables:
SNIPDATA_LAKE (default ./lake), SNIPDATA_WAREHOUSE (default ./warehouse/snip.duckdb),
SNIP_DATABASE_URL for extract.
"""

from __future__ import annotations

import argparse
import os
import sys
from datetime import UTC, date, datetime, timedelta
from pathlib import Path

import duckdb

from snipdata import extract as ex
from snipdata import generate as gen
from snipdata import lake as lk
from snipdata import orchestrate as orc
from snipdata.transform import run_dbt

DEFAULT_DSN = "postgres://snip:snip@localhost:5432/snip?sslmode=disable"


def default_warehouse() -> Path:
    return Path(os.environ.get("SNIPDATA_WAREHOUSE", "warehouse/snip.duckdb"))


def cmd_generate(args: argparse.Namespace) -> int:
    opts = gen.Options(
        seed=args.seed,
        days=args.days,
        end=args.end,
        links=args.links,
        clicks_per_day=args.clicks_per_day,
        duplicate_rate=args.duplicate_rate,
    )
    s = gen.generate(args.lake, opts)
    print(
        f"generated {s.clicks:,} clicks (+{s.duplicates:,} redelivered copies) on {s.links} links "
        f"({s.deleted_links} deleted), {s.first_day} to {s.last_day}, into {args.lake}/"
    )
    return 0


def cmd_extract(args: argparse.Namespace) -> int:
    s = ex.extract(args.dsn, args.lake, args.day, args.settle_seconds)
    day = f"dt={s.snapshot_day}"
    print(f"links:        {s.links:>6} rows -> {args.lake}/links/{day}/snapshot.parquet")
    print(f"owners:       {s.owners:>6} rows -> {args.lake}/owners/{day}/snapshot.parquet")
    print(
        f"audit_events: {s.audit_events:>6} new rows (watermark {s.watermark_before} -> "
        f"{s.watermark_after})"
    )
    return 0


def cmd_dbt(args: argparse.Namespace, dbt_args: list[str]) -> int:
    command = ["build", *dbt_args] if args.command == "build" else dbt_args
    out = run_dbt(args.lake, args.warehouse, args.dbt_dir, command)
    return 0 if out.success else 1


def orc_config(args: argparse.Namespace) -> orc.Config:
    return orc.Config(
        lake=args.lake,
        warehouse=args.warehouse,
        dbt_dir=args.dbt_dir,
        dsn=args.dsn,
        grace=timedelta(minutes=args.grace_minutes),
        wait=timedelta(minutes=args.wait_minutes),
        retries=args.retries,
    )


def cmd_run(args: argparse.Namespace) -> int:
    day = args.day or datetime.now(UTC).date() - timedelta(days=1)
    cfg = orc_config(args)
    with orc.exclusive(cfg):
        return 0 if orc.run_day(cfg, day) else 1


def cmd_backfill(args: argparse.Namespace) -> int:
    cfg = orc_config(args)
    with orc.exclusive(cfg):
        failed = orc.backfill(cfg, args.start, args.end, args.missing_only)
    total = len(orc.days(args.start, args.end))
    print(f"backfill {args.start} to {args.end}: {total - len(failed)} of {total} days done")
    if failed:
        print("failed: " + " ".join(d.isoformat() for d in failed), file=sys.stderr)
    return 1 if failed else 0


def cmd_runs(args: argparse.Namespace) -> int:
    cfg = orc_config(args)
    runs = orc.latest_runs(cfg)
    if not runs:
        print(f"no runs yet ({cfg.run_log} is empty)")
        return 0
    print(f"{'day':<11} {'run':<8} {'status':<7} {'step':<15} {'retries':>7}  message")
    for day, entries in list(runs.items())[-args.last :]:
        last = entries[-1]
        ok = last["status"] == "ok" and last["step"] == orc.STEPS[-1][0]
        retries = sum(1 for e in entries if e["status"] == "retry")
        print(
            f"{day.isoformat():<11} {last['run']:<8} {'ok' if ok else last['status']:<7} "
            f"{last['step']:<15} {retries:>7}  {last['message']}"
        )
    return 0


def connect(lake: Path, warehouse: Path | None) -> duckdb.DuckDBPyConnection:
    """An in-memory DuckDB with a view per lake table, and the warehouse attached as wh."""
    con = duckdb.connect()
    con.execute("SET TimeZone = 'UTC'")
    for t in lk.TABLES:
        if lk.has_files(lake, t):
            con.execute(
                f"CREATE VIEW {t} AS SELECT * FROM read_parquet('{lk.glob(lake, t)}', "
                "hive_partitioning = true, union_by_name = true)"
            )
    if warehouse is not None and warehouse.exists():
        con.execute(f"ATTACH '{warehouse.as_posix()}' AS wh (READ_ONLY)")
    return con


def cmd_query(args: argparse.Namespace) -> int:
    con = connect(args.lake, args.warehouse)
    rel = con.sql(args.sql)
    if rel is not None:  # a statement like SET returns no rows
        rel.show(max_rows=args.max_rows, max_width=200)
    return 0


def parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="snipdata", description=__doc__.split("\n\n")[0] if __doc__ else ""
    )
    p.add_argument(
        "--lake",
        type=Path,
        default=lk.default_lake(),
        help="the lake folder (default: %(default)s)",
    )
    sub = p.add_subparsers(dest="command", required=True)

    g = sub.add_parser("generate", help="write realistic, seeded click data into the lake")
    g.add_argument("--seed", type=int, default=42)
    g.add_argument("--days", type=int, default=30)
    g.add_argument(
        "--end",
        type=date.fromisoformat,
        default=None,
        help="last day, YYYY-MM-DD (default: today, UTC)",
    )
    g.add_argument("--links", type=int, default=25)
    g.add_argument("--clicks-per-day", type=int, default=5000)
    g.add_argument("--duplicate-rate", type=float, default=0.005)

    e = sub.add_parser("extract", help="copy snip's links, owners and audit events from Postgres")
    e.add_argument("--dsn", default=os.environ.get("SNIP_DATABASE_URL", DEFAULT_DSN))
    e.add_argument(
        "--day", type=date.fromisoformat, default=None, help="snapshot day (default: today, UTC)"
    )
    e.add_argument("--settle-seconds", type=float, default=ex.DEFAULT_SETTLE_SECONDS)

    for name, text in [
        ("build", "dbt build: every model and its tests; extra arguments go to dbt"),
        ("dbt", "any dbt command, e.g. snipdata dbt docs generate"),
    ]:
        b = sub.add_parser(name, help=text)
        b.add_argument("--warehouse", type=Path, default=default_warehouse())
        b.add_argument("--dbt-dir", type=Path, default=Path("dbt"))

    r = sub.add_parser("run", help="run the pipeline for one day: sensor, extract, dbt, verify")
    r.add_argument(
        "--day", type=date.fromisoformat, default=None, help="YYYY-MM-DD (default: yesterday, UTC)"
    )
    bf = sub.add_parser("backfill", help="run the pipeline for every day in a range, oldest first")
    bf.add_argument("--start", type=date.fromisoformat, required=True)
    bf.add_argument("--end", type=date.fromisoformat, required=True)
    bf.add_argument(
        "--missing-only", action="store_true", help="skip days whose latest run succeeded"
    )
    rs = sub.add_parser("runs", help="show each day's latest run from the run log")
    rs.add_argument("--last", type=int, default=31, help="how many days to show")
    for sp in (r, bf, rs):
        sp.add_argument("--warehouse", type=Path, default=default_warehouse())
        sp.add_argument("--dbt-dir", type=Path, default=Path("dbt"))
        sp.add_argument("--dsn", default=os.environ.get("SNIP_DATABASE_URL", DEFAULT_DSN))
        sp.add_argument("--retries", type=int, default=2, help="extra attempts for retryable steps")
        sp.add_argument(
            "--grace-minutes", type=float, default=60, help="how late a click may arrive"
        )
        sp.add_argument(
            "--wait-minutes", type=float, default=0, help="how long the sensor keeps checking"
        )

    q = sub.add_parser("query", help="run SQL over the lake; the warehouse is attached as wh")
    q.add_argument("sql")
    q.add_argument("--warehouse", type=Path, default=default_warehouse())
    q.add_argument("--max-rows", type=int, default=40)
    return p


def main(argv: list[str] | None = None) -> int:
    args, rest = parser().parse_known_args(argv)
    if args.command in ("build", "dbt"):
        return cmd_dbt(args, rest)
    if rest:
        print(f"snipdata: unrecognised arguments: {' '.join(rest)}", file=sys.stderr)
        return 2
    try:
        if args.command == "generate":
            return cmd_generate(args)
        if args.command == "extract":
            return cmd_extract(args)
        if args.command == "run":
            return cmd_run(args)
        if args.command == "backfill":
            return cmd_backfill(args)
        if args.command == "runs":
            return cmd_runs(args)
        return cmd_query(args)
    except (RuntimeError, ValueError, duckdb.Error) as err:
        print(f"snipdata: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
