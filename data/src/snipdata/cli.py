"""snipdata: one command for each step of the pipeline.

    uv run snipdata generate             # fake but realistic data, no snip needed
    uv run snipdata extract              # or: copy snip's Postgres into the lake
    uv run snipdata build                # dbt build: models + tests, into the warehouse
    uv run snipdata dbt docs generate    # any other dbt command
    uv run snipdata query "SELECT ..."   # SQL over the lake (and the warehouse, as wh.*)

Run it from the data/ folder. Paths come from flags or environment variables:
SNIPDATA_LAKE (default ./lake), SNIPDATA_WAREHOUSE (default ./warehouse/snip.duckdb),
SNIP_DATABASE_URL for extract.
"""

from __future__ import annotations

import argparse
import os
import sys
from datetime import date
from pathlib import Path

import duckdb

from snipdata import extract as ex
from snipdata import generate as gen
from snipdata import lake as lk

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
    # Imported here: dbt takes a second to load, and the other commands don't need it.
    from dbt.cli.main import dbtRunner

    args.warehouse.parent.mkdir(parents=True, exist_ok=True)
    os.environ["SNIPDATA_LAKE"] = str(args.lake.resolve())
    os.environ["SNIPDATA_WAREHOUSE"] = str(args.warehouse.resolve())
    project = str(args.dbt_dir)
    command = ["build", *dbt_args] if args.command == "build" else dbt_args
    result = dbtRunner().invoke([*command, "--project-dir", project, "--profiles-dir", project])
    return 0 if result.success else 1


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
        return cmd_query(args)
    except (RuntimeError, ValueError, duckdb.Error) as err:
        print(f"snipdata: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
