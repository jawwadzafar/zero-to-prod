from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

import duckdb
import pyarrow as pa
import pyarrow.parquet as pq
import pytest

from clickstream import lake
from clickstream.config import Config
from clickstream.events import BadEvent, Click, id_key, parse


def at(ms: int) -> datetime:
    return datetime.fromtimestamp(ms // 1000, tz=UTC).replace(microsecond=ms % 1000 * 1000)


# --- events ---------------------------------------------------------------------


def test_parse_uses_snips_fields() -> None:
    c = parse("1793282586300-0", {"slug": "go", "at": "1793282586265"})
    assert c == Click("1793282586300-0", "go", at(1793282586265))
    assert c.clicked_at.microsecond == 265_000  # exact milliseconds, no float rounding


def test_parse_falls_back_to_the_id_time() -> None:
    assert parse("1793282586300-4", {"slug": "go"}).clicked_at == at(1793282586300)


@pytest.mark.parametrize(
    "fields",
    [
        {"at": "1"},  # no slug
        {"slug": "", "at": "1"},
        {"slug": "go", "at": "yesterday"},
        {"slug": "go", "at": "-5"},
    ],
)
def test_parse_rejects_poison(fields: dict[str, str]) -> None:
    with pytest.raises(BadEvent):
        parse("1-0", fields)


def test_ids_sort_as_numbers() -> None:
    ids = ["10-0", "9-1", "9-0", "100-0"]
    assert sorted(ids, key=id_key) == ["9-0", "9-1", "10-0", "100-0"]


def test_config_from_env() -> None:
    cfg = Config.from_env(
        {"SNIP_REDIS_ADDR": "redis:6379", "SNIP_REDIS_PASSWORD": "pw", "LAKE_DIR": "/lake"}
    )
    assert (cfg.redis_host, cfg.redis_port, cfg.redis_password) == ("redis", 6379, "pw")
    assert cfg.lake_dir == Path("/lake")


# --- lake -----------------------------------------------------------------------

DAY1 = 1793232000000  # 2026-10-29T00:00:00Z
DAY2 = DAY1 + 86_400_000


def clicks(*specs: tuple[str, str, int]) -> list[Click]:
    return [Click(i, s, at(ms)) for i, s, ms in specs]


def test_write_batch_follows_the_contract(tmp_path: Path) -> None:
    batch = clicks(("10-0", "go", DAY1 + 5), ("9-0", "py", DAY1 + 1), ("11-0", "go", DAY2 + 7))
    paths = lake.write_batch(tmp_path, batch)

    assert [p.relative_to(tmp_path).as_posix() for p in paths] == [
        "clicks/dt=2026-10-29/stream-9-0_10-0.parquet",  # first/last ID, numeric order
        "clicks/dt=2026-10-30/stream-11-0_11-0.parquet",  # event time decides the day
    ]
    t = pq.read_table(paths[0])
    assert t.schema.field("event_id").type == pa.string()
    assert t.schema.field("clicked_at").type == pa.timestamp("ms", tz="UTC")
    assert t.column("event_id").to_pylist() == ["9-0", "10-0"]


def test_rewriting_a_batch_replaces_its_file(tmp_path: Path) -> None:
    batch = clicks(("1-0", "go", DAY1), ("2-0", "go", DAY1), ("2-0", "go", DAY1))
    first = lake.write_batch(tmp_path, batch)
    again = lake.write_batch(tmp_path, batch)  # redelivered after a crash
    assert first == again
    files = list((tmp_path / "clicks").rglob("*"))
    assert [f.name for f in files if f.is_file()] == ["stream-1-0_2-0.parquet"]  # no .tmp left
    glob = str(tmp_path / "clicks" / "dt=*" / "*.parquet")
    rows = duckdb.sql(
        f"SELECT count(*), count(DISTINCT event_id), min(dt) "
        f"FROM read_parquet('{glob}', hive_partitioning = true)"
    ).fetchall()
    assert rows == [(2, 2, datetime(2026, 10, 29).date())]  # dt comes from the folder name


def test_remove_leftovers(tmp_path: Path) -> None:
    d = lake.partition_dir(tmp_path, at(DAY1).date())
    d.mkdir(parents=True)
    (d / ".stream-1-0_2-0.parquet.42.tmp").write_bytes(b"half a file")
    (d / "stream-1-0_2-0.parquet").write_bytes(b"kept")
    assert lake.remove_leftovers(tmp_path) == 1
    assert [p.name for p in d.iterdir()] == ["stream-1-0_2-0.parquet"]
