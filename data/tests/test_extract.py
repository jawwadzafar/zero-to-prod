"""extract against a real Postgres with snip's real schema (its migration files).

Set SNIPDATA_TEST_DATABASE_URL to run these, e.g.
postgres://snip:snip@localhost:5432/snip?sslmode=disable. Each test works in a
fresh schema, so it never touches snip's own tables.
"""

from __future__ import annotations

import os
import secrets
from collections.abc import Iterator
from datetime import date
from pathlib import Path
from typing import Any

import duckdb
import psycopg
import pyarrow.parquet as pq
import pytest
from psycopg.conninfo import make_conninfo

from snipdata import lake as lk
from snipdata.extract import audit_watermark, extract
from snipdata.generate import Options, generate

DSN = os.environ.get("SNIPDATA_TEST_DATABASE_URL")
MIGRATIONS = Path(__file__).parents[2] / "snip" / "internal" / "store" / "postgres" / "migrations"
DAY = date(2026, 10, 29)

needs_db = pytest.mark.skipif(not DSN, reason="SNIPDATA_TEST_DATABASE_URL not set")


@pytest.fixture
def db() -> Iterator[tuple[str, psycopg.Connection[Any]]]:
    """(dsn, connection) for a fresh schema holding snip's tables."""
    assert DSN
    schema = f"snipdata_test_{secrets.token_hex(4)}"
    with psycopg.connect(DSN, autocommit=True) as admin:
        admin.execute(f"CREATE SCHEMA {schema}")
        dsn = make_conninfo(DSN, options=f"-c search_path={schema}")
        try:
            with psycopg.connect(dsn, autocommit=True) as conn:
                for f in sorted(MIGRATIONS.glob("*.sql")):
                    conn.execute(f.read_text())  # type: ignore[arg-type]  # trusted files from this repo
                yield dsn, conn
        finally:
            admin.execute(f"DROP SCHEMA {schema} CASCADE")


def add_key(conn: psycopg.Connection[Any], name: str) -> int:
    row = conn.execute(
        "INSERT INTO api_keys (name, key_hash) VALUES (%s, %s) RETURNING id",
        (name, secrets.token_hex(32)),
    ).fetchone()
    assert row
    conn.execute(
        "INSERT INTO audit_events (owner_id, action, detail) VALUES (%s, 'key.created', %s)",
        (row[0], f'{{"name": "{name}"}}'),
    )
    return int(row[0])


def add_link(conn: psycopg.Connection[Any], owner: int, slug: str, url: str) -> None:
    conn.execute("INSERT INTO links (slug, url, owner_id) VALUES (%s, %s, %s)", (slug, url, owner))
    conn.execute(
        "INSERT INTO audit_events (owner_id, action, slug, detail)"
        " VALUES (%s, 'link.created', %s, %s)",
        (owner, slug, f'{{"url": "{url}"}}'),
    )


def count(lake: Path, table: str) -> int:
    row = duckdb.execute(f"SELECT count(*) FROM read_parquet('{lk.glob(lake, table)}')").fetchone()
    return int(row[0]) if row else 0


@needs_db
def test_snapshot_and_incremental_audit(
    db: tuple[str, psycopg.Connection[Any]], tmp_path: Path
) -> None:
    dsn, conn = db
    owner = add_key(conn, "marketing")
    add_link(conn, owner, "golang", "https://go.dev/")
    add_link(conn, owner, "pgdocs", "https://www.postgresql.org/docs/")

    s1 = extract(dsn, tmp_path, DAY, settle_seconds=0)
    assert (s1.links, s1.owners, s1.audit_events) == (2, 1, 3)
    assert (s1.watermark_before, s1.watermark_after) == (0, 3)

    # The lake never holds a secret.
    owners_file = lk.partition_dir(tmp_path, "owners", DAY) / "snapshot.parquet"
    assert "key_hash" not in pq.read_schema(owners_file).names

    # A rerun with nothing new copies nothing and changes nothing.
    s2 = extract(dsn, tmp_path, DAY, settle_seconds=0)
    assert s2.audit_events == 0 and s2.watermark_after == 3
    assert count(tmp_path, "audit_events") == 3

    # New rows: only those are appended; the same-day snapshot is replaced, not duplicated.
    add_link(conn, owner, "duckdb", "https://duckdb.org/docs/")
    conn.execute("UPDATE links SET clicks = clicks + 5 WHERE slug = 'golang'")
    s3 = extract(dsn, tmp_path, DAY, settle_seconds=0)
    assert (s3.links, s3.audit_events, s3.watermark_before, s3.watermark_after) == (3, 1, 3, 4)
    assert count(tmp_path, "audit_events") == 4
    assert count(tmp_path, "links") == 3
    clicks = duckdb.execute(
        f"SELECT clicks FROM read_parquet('{lk.glob(tmp_path, 'links')}') WHERE slug = 'golang'"
    ).fetchall()
    assert clicks == [(5,)]

    # The next day's snapshot sits next to today's: history is kept.
    extract(dsn, tmp_path, date(2026, 10, 30), settle_seconds=0)
    assert count(tmp_path, "links") == 6


@needs_db
def test_settle_time_holds_back_fresh_rows(
    db: tuple[str, psycopg.Connection[Any]], tmp_path: Path
) -> None:
    dsn, conn = db
    add_key(conn, "marketing")
    s = extract(dsn, tmp_path, DAY, settle_seconds=3600)
    assert s.audit_events == 0 and audit_watermark(tmp_path) == 0


def test_refuses_a_generated_lake(tmp_path: Path) -> None:
    generate(tmp_path, Options(days=1, end=DAY, links=3, clicks_per_day=10))
    with pytest.raises(RuntimeError, match="generated data"):
        extract("postgres://unused", tmp_path, DAY)


def test_watermark_of_empty_lake_is_zero(tmp_path: Path) -> None:
    assert audit_watermark(tmp_path) == 0
