from datetime import date
from pathlib import Path

import duckdb
import pyarrow.parquet as pq
import pytest

from snipdata import lake as lk
from snipdata.generate import Options, generate

OPTS = Options(seed=7, days=10, end=date(2026, 10, 29), links=25, clicks_per_day=800)


def files(lake: Path) -> dict[str, bytes]:
    return {p.relative_to(lake).as_posix(): p.read_bytes() for p in sorted(lake.rglob("*.parquet"))}


def test_same_seed_same_files(tmp_path: Path) -> None:
    generate(tmp_path / "a", OPTS)
    generate(tmp_path / "b", OPTS)
    assert files(tmp_path / "a") == files(tmp_path / "b")


def test_rerun_replaces_instead_of_adding(tmp_path: Path) -> None:
    lake = tmp_path / "lake"
    generate(lake, Options(**{**OPTS.__dict__, "days": 20}))
    generate(lake, OPTS)  # fewer days: the older partitions must go
    days = sorted(p.name for p in (lake / "clicks").iterdir())
    assert days[0] == "dt=2026-10-20" and days[-1] == "dt=2026-10-29" and len(days) == 10


def test_follows_the_clicks_contract(tmp_path: Path) -> None:
    generate(tmp_path, OPTS)
    for path in (tmp_path / "clicks").glob("dt=*/*.parquet"):
        schema = pq.read_schema(path)
        assert schema.names == ["event_id", "slug", "clicked_at"]
        assert str(schema.field("clicked_at").type) == "timestamp[ms, tz=UTC]"
        # every event in a dt= folder happened on that UTC day
        day = path.parent.name.removeprefix("dt=")
        days = duckdb.execute(
            f"SELECT DISTINCT strftime(clicked_at AT TIME ZONE 'UTC', '%Y-%m-%d') "
            f"FROM read_parquet('{path}')"
        ).fetchall()
        assert days == [(day,)]


def test_counts_and_duplicates(tmp_path: Path) -> None:
    s = generate(tmp_path, OPTS)
    clicks = lk.glob(tmp_path, "clicks")
    total, distinct = duckdb.execute(
        f"SELECT count(*), count(DISTINCT event_id) FROM read_parquet('{clicks}')"
    ).fetchone() or (0, 0)
    assert distinct == s.clicks
    assert total == s.clicks + s.duplicates
    assert s.duplicates > 0  # 0.5% of ~7,000 events


def test_snapshot_counter_matches_clicks(tmp_path: Path) -> None:
    generate(tmp_path, OPTS)
    mismatched = duckdb.execute(f"""
        WITH c AS (
            SELECT slug, count(DISTINCT event_id) AS n
            FROM read_parquet('{lk.glob(tmp_path, "clicks")}') GROUP BY slug
        )
        SELECT l.slug, l.clicks, c.n
        FROM read_parquet('{lk.glob(tmp_path, "links")}') AS l LEFT JOIN c USING (slug)
        WHERE l.clicks <> coalesce(c.n, 0)
    """).fetchall()
    assert mismatched == []


def test_no_click_outside_a_links_life(tmp_path: Path) -> None:
    generate(tmp_path, OPTS)
    bad = duckdb.execute(f"""
        WITH ev AS (SELECT * FROM read_parquet('{lk.glob(tmp_path, "audit_events")}')),
        life AS (
            SELECT slug,
                   max("at") FILTER (action = 'link.created') AS created,
                   max("at") FILTER (action = 'link.deleted') AS deleted
            FROM ev WHERE slug IS NOT NULL GROUP BY slug
        )
        SELECT c.* FROM read_parquet('{lk.glob(tmp_path, "clicks")}') AS c JOIN life USING (slug)
        WHERE c.clicked_at < life.created OR c.clicked_at >= life.deleted
    """).fetchall()
    assert bad == []


def test_refuses_to_overwrite_real_data(tmp_path: Path) -> None:
    generate(tmp_path, OPTS)
    (tmp_path / lk.GENERATED_MARKER).unlink()  # now it looks like an extracted lake
    with pytest.raises(RuntimeError, match="wasn't generated"):
        generate(tmp_path, OPTS)
