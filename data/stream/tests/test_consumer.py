"""The consumer against a real Redis (the stream commands are the point).

Start one and point the tests at it:

    docker run -d --name stream-test -p 6379:6379 redis:7-alpine
    TEST_REDIS_URL=redis://localhost:6379/15 uv run pytest

Without TEST_REDIS_URL these tests are skipped. They flush the database they use.
"""

from __future__ import annotations

import json
import os
from collections.abc import Iterator
from pathlib import Path
from typing import Any

import duckdb
import pytest
import redis

from clickstream.config import Config
from clickstream.consumer import GROUP, STREAM, BatchResult, Consumer

URL = os.environ.get("TEST_REDIS_URL", "")
pytestmark = pytest.mark.skipif(not URL, reason="set TEST_REDIS_URL to a Redis you can flush")


@pytest.fixture
def r() -> Iterator[redis.Redis]:
    client = redis.Redis.from_url(URL, decode_responses=True)
    client.flushdb()
    yield client
    client.flushdb()


def make(r: redis.Redis, lake: Path, name: str = "a1", **kw: Any) -> Consumer:
    cfg = Config(lake_dir=lake, consumer=name, flush_seconds=0.2, **kw)
    c = Consumer(r, cfg)
    c.ensure_group()
    return c


def click(r: redis.Redis, slug: str, at: int = 1793282586265, maxlen: int | None = None) -> str:
    """What snip's API does on every redirect."""
    return str(r.xadd(STREAM, {"slug": slug, "at": at}, maxlen=maxlen, approximate=False))


def lake_rows(lake: Path) -> tuple[int, int]:
    glob = lake / "clicks" / "dt=*" / "*.parquet"
    [(n, distinct)] = duckdb.sql(
        f"SELECT count(*), count(DISTINCT event_id) FROM read_parquet('{glob}')"
    ).fetchall()
    return int(n), int(distinct)


def drain(c: Consumer) -> list[BatchResult]:
    out = []
    while (res := c.process_once()).events:
        out.append(res)
    return out


def test_every_event_lands_once_and_is_acknowledged(r: redis.Redis, tmp_path: Path) -> None:
    c = make(r, tmp_path, batch_size=4)
    ids = [click(r, s) for s in ["go", "py", "go", "rs", "go", "go"]]
    results = drain(c)
    assert [res.events for res in results] == [4, 2]
    assert lake_rows(tmp_path) == (6, 6)
    assert r.xpending(STREAM, GROUP)["pending"] == 0
    assert results[0].files[0].name == f"stream-{ids[0]}_{ids[3]}.parquet"


def test_snips_workers_still_see_every_event(r: redis.Redis, tmp_path: Path) -> None:
    r.xgroup_create(STREAM, "workers", id="0", mkstream=True)  # snip's click counters
    c = make(r, tmp_path)
    for _ in range(3):
        click(r, "go")
    drain(c)
    got = r.xreadgroup("workers", "w1", {STREAM: ">"})
    assert len(got[0][1]) == 3  # a separate group has its own place in the stream


def test_crash_before_ack_means_redelivery_not_duplicates(
    r: redis.Redis, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    c = make(r, tmp_path, batch_size=10)
    for s in ["go", "py", "go"]:
        click(r, s)

    def crash(*_args: object) -> None:
        raise RuntimeError("killed after the file was written, before XACK")

    monkeypatch.setattr(r, "xack", crash)
    with pytest.raises(RuntimeError):
        c.process_once()
    monkeypatch.undo()
    assert r.xpending(STREAM, GROUP)["pending"] == 3  # Redis still owes us these
    assert lake_rows(tmp_path) == (3, 3)

    click(r, "rs")  # life goes on while we were down
    restarted = make(r, tmp_path)  # same name, as after a container restart
    first = restarted.process_once()
    assert (first.source, first.events) == ("pending", 3)  # our own pending events come first
    drain(restarted)
    assert lake_rows(tmp_path) == (4, 4)  # same file rewritten, not a second copy
    assert r.xpending(STREAM, GROUP)["pending"] == 0


def test_a_replacement_consumer_claims_a_dead_ones_events(
    r: redis.Redis, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    dead = make(r, tmp_path, name="pod-abc")
    for _ in range(3):
        click(r, "go")
    monkeypatch.setattr(r, "xack", lambda *_a: (_ for _ in ()).throw(RuntimeError("crash")))
    with pytest.raises(RuntimeError):
        dead.process_once()
    monkeypatch.undo()

    patient = make(r, tmp_path, name="pod-xyz", claim_idle_seconds=60)
    assert patient.process_once().events == 0  # too soon: pod-abc might still be working

    eager = make(r, tmp_path, name="pod-xyz", claim_idle_seconds=0)
    res = eager.process_once()
    assert (res.source, res.events) == ("claimed", 3)
    assert lake_rows(tmp_path) == (3, 3)


def test_a_batch_is_written_when_the_oldest_event_has_waited_long_enough(
    r: redis.Redis, tmp_path: Path
) -> None:
    now = [0.0]

    def clock() -> float:
        now[0] += 0.1  # each look at the clock moves time on 100 ms
        return now[0]

    c = Consumer(r, Config(lake_dir=tmp_path, consumer="a1", batch_size=1000, flush_seconds=1.0))
    c.clock = clock
    c.ensure_group()
    click(r, "go")
    res = c.process_once()  # one event, far from a full batch, still written after ~1 s
    assert (res.events, len(res.files)) == (1, 1)


def test_poison_events_are_skipped_and_acknowledged(r: redis.Redis, tmp_path: Path) -> None:
    c = make(r, tmp_path)
    click(r, "go")
    r.xadd(STREAM, {"at": "1"})  # no slug
    r.xadd(STREAM, {"slug": "go", "at": "next tuesday"})
    res = c.process_once()
    assert (res.events, res.written, res.bad) == (3, 1, 2)
    assert r.xpending(STREAM, GROUP)["pending"] == 0


def test_trimmed_unread_events_are_reported_as_a_gap(r: redis.Redis, tmp_path: Path) -> None:
    c = make(r, tmp_path, batch_size=2)
    for _ in range(5):
        click(r, "go")
    assert c.process_once().events == 2  # we read 2 of 5, then fall behind...
    for _ in range(8):
        click(r, "go")
    r.xtrim(STREAM, maxlen=4, approximate=False)  # ...and the cap trims the oldest 9

    gap = c.check_gap()
    assert gap is not None and gap.lost == 7  # 13 added - 2 read - 4 still there
    drain(c)
    assert lake_rows(tmp_path) == (6, 6)
    assert c.check_gap() is None  # reported once, then we're past it

    records = (tmp_path / "_gaps" / "clicks.jsonl").read_text().splitlines()
    assert [json.loads(line)["lost"] for line in records] == [7]


def test_a_new_group_on_a_trimmed_stream_reports_what_it_missed(
    r: redis.Redis, tmp_path: Path
) -> None:
    for _ in range(10):
        click(r, "go", maxlen=4)  # snip's MAXLEN, scaled down
    gap = make(r, tmp_path).check_gap()
    assert gap is not None and gap.lost == 6


def test_no_gap_when_keeping_up(r: redis.Redis, tmp_path: Path) -> None:
    c = make(r, tmp_path)
    for _ in range(10):
        click(r, "go", maxlen=4)
        assert c.process_once().events == 1
    assert c.check_gap() is None
    assert c.lag() == 0


def test_pending_events_trimmed_before_the_ack_are_counted(
    r: redis.Redis, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    c = make(r, tmp_path, batch_size=10)
    for _ in range(3):
        click(r, "go")
    monkeypatch.setattr(r, "xack", lambda *_a: (_ for _ in ()).throw(RuntimeError("crash")))
    with pytest.raises(RuntimeError):
        c.process_once()
    monkeypatch.undo()
    click(r, "go")
    r.xtrim(STREAM, maxlen=1, approximate=False)  # the 3 pending events are trimmed

    res = c.process_once()
    assert (res.source, res.events, res.trimmed) == ("pending", 3, 3)
    assert r.xpending(STREAM, GROUP)["pending"] == 0
