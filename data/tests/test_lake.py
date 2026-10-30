from datetime import date
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

from snipdata import lake as lk


def test_partition_dir_is_hive_style(tmp_path: Path) -> None:
    assert lk.partition_dir(tmp_path, "clicks", date(2026, 10, 29)) == (
        tmp_path / "clicks" / "dt=2026-10-29"
    )


def test_atomic_write_replaces_and_leaves_no_temp_file(tmp_path: Path) -> None:
    path = tmp_path / "t" / "dt=2026-10-29" / "x.parquet"
    lk.write_parquet_atomic(pa.table({"n": [1, 2]}), path)
    lk.write_parquet_atomic(pa.table({"n": [3]}), path)
    assert pq.read_table(path).column("n").to_pylist() == [3]
    assert [p.name for p in path.parent.iterdir()] == ["x.parquet"]


def test_has_files(tmp_path: Path) -> None:
    assert not lk.has_files(tmp_path, "clicks")
    lk.write_parquet_atomic(
        pa.table({"n": [1]}), lk.partition_dir(tmp_path, "clicks", date.today()) / "a.parquet"
    )
    assert lk.has_files(tmp_path, "clicks")
