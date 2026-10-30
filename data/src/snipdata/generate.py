"""A seeded generator of realistic snip data, for readers without a running snip (and for tests).

It writes the same lake layout as `extract` and the streaming consumer:
owners and a links snapshot (as if extracted on the last day), the audit
events that created and deleted those links, and click events partitioned by
day. The same seed and options always produce the same rows.

What makes it realistic enough to analyse:
- popularity follows a long tail (a few links get most clicks, most get few);
- traffic dips at weekends and peaks in the afternoon (UTC);
- one link launches mid-window and spikes, then decays;
- a few links are deleted, after collecting some clicks;
- a small share of events is delivered twice, the way an at-least-once
  stream consumer redelivers after a crash (chapter 17.6). Their event_id
  repeats, which is how the pipeline removes them.
"""

from __future__ import annotations

import json
import math
import random
import shutil
from collections import defaultdict
from dataclasses import dataclass
from datetime import UTC, date, datetime, time, timedelta
from pathlib import Path

import pyarrow as pa

from snipdata import lake as lk

OWNER_NAMES = ["marketing", "docs-team", "growth", "alice-laptop", "release-bot"]

# Real public pages, so the data reads like something you'd shorten.
TARGETS = [
    ("golang", "https://go.dev/"),
    ("go-tour", "https://go.dev/tour/"),
    ("pgdocs", "https://www.postgresql.org/docs/"),
    ("redis-streams", "https://redis.io/docs/latest/develop/data-types/streams/"),
    ("duckdb", "https://duckdb.org/docs/"),
    ("parquet", "https://parquet.apache.org/docs/"),
    ("dbt", "https://docs.getdbt.com/"),
    ("k8s", "https://kubernetes.io/docs/home/"),
    ("docker", "https://docs.docker.com/"),
    ("python", "https://www.python.org/"),
    ("uv", "https://docs.astral.sh/uv/"),
    ("mdn-http", "https://developer.mozilla.org/en-US/docs/Web/HTTP"),
    ("rfc9110", "https://www.rfc-editor.org/rfc/rfc9110"),
    ("sre-book", "https://sre.google/sre-book/table-of-contents/"),
    ("12factor", "https://12factor.net/"),
    ("owasp-top10", "https://owasp.org/www-project-top-ten/"),
    ("prometheus", "https://prometheus.io/docs/introduction/overview/"),
    ("grafana", "https://grafana.com/docs/"),
    ("terraform", "https://developer.hashicorp.com/terraform/docs"),
    ("git-book", "https://git-scm.com/book/en/v2"),
    ("arrow", "https://arrow.apache.org/docs/"),
    ("kimball", "https://www.kimballgroup.com/data-warehouse-business-intelligence-resources/"),
    ("wikipedia", "https://en.wikipedia.org/wiki/URL_shortening"),
    ("launch", "https://blog.example.com/we-launched"),
    ("pricing", "https://www.example.com/pricing"),
    ("careers", "https://www.example.com/careers"),
    ("webinar", "https://events.example.com/webinar-data"),
    ("newsletter", "https://news.example.com/issue-42"),
    ("status", "https://status.example.com/"),
    ("handbook", "https://jawwadzafar.github.io/zero-to-prod/"),
]

# Share of each day's clicks per UTC hour: quiet at night, busiest mid-afternoon.
HOUR_WEIGHTS = [max(0.15, math.exp(-(((h - 14) / 5.0) ** 2))) for h in range(24)]


@dataclass(frozen=True)
class Options:
    seed: int = 42
    days: int = 30  # how many days of clicks, ending on `end`
    end: date | None = None  # last day (UTC); default yesterday, the last full day
    links: int = 25
    clicks_per_day: int = 5000  # average, before weekend dips and the launch spike
    duplicate_rate: float = 0.005  # share of events delivered twice


@dataclass(frozen=True)
class Summary:
    days: int
    links: int
    deleted_links: int
    clicks: int  # distinct events
    duplicates: int  # extra copies written
    first_day: date
    last_day: date


@dataclass
class _Link:
    slug: str
    url: str
    owner_id: int
    created_at: datetime
    deleted_at: datetime | None
    weight: float
    clicks: int = 0


def _ms(dt: datetime) -> int:
    return int(dt.timestamp() * 1000)


def generate(lake: Path, opts: Options) -> Summary:
    """Write a generated lake at `lake`, replacing any lake it generated before."""
    if opts.days < 1 or opts.links < 1 or opts.clicks_per_day < 0:
        raise ValueError("days and links must be at least 1, clicks_per_day at least 0")
    if not 0 <= opts.duplicate_rate < 1:
        raise ValueError("duplicate_rate must be in [0, 1)")
    _prepare(lake)

    rng = random.Random(opts.seed)
    end = opts.end or datetime.now(UTC).date() - timedelta(days=1)  # no clicks from the future
    first = end - timedelta(days=opts.days - 1)
    window_start = datetime.combine(first, time(), UTC)
    extracted_at = datetime.combine(end, time(23, 59, 59), UTC)

    # Owners, created well before the window.
    owners = [
        (i + 1, name, window_start - timedelta(days=90 - 7 * i))
        for i, name in enumerate(OWNER_NAMES)
    ]

    # Links: names from TARGETS, then numbered copies if more are asked for.
    links: list[_Link] = []
    for i in range(opts.links):
        slug, url = TARGETS[i % len(TARGETS)]
        if i >= len(TARGETS):
            slug = f"{slug}-{i // len(TARGETS) + 1}"
        # Two thirds exist before the window; the rest are created during it.
        if rng.random() < 2 / 3:
            created = window_start - timedelta(days=rng.uniform(1, 60))
        else:
            created = window_start + timedelta(days=rng.uniform(0, opts.days * 0.8))
        weight = 1.0 / (i + 1) ** 1.1  # long tail: link 1 gets the most traffic
        links.append(_Link(slug, url, rng.randint(1, len(owners)), created, None, weight))
    rng.shuffle(links)  # so popularity isn't in TARGETS order

    # The launch: one link created a few days in, with a spike that decays.
    launch = next((x for x in links if x.slug == "launch"), links[0])
    launch.created_at = window_start + timedelta(days=min(5, opts.days - 1), hours=9)

    # Deletions: a few links stop part-way through (only if there are enough links).
    for link in links[: max(0, opts.links // 10)]:
        if link is launch:
            continue
        start = max(link.created_at, window_start)
        link.deleted_at = start + (datetime.combine(end, time(), UTC) - start) * rng.uniform(
            0.3, 0.8
        )

    duplicates = 0
    total = 0
    for offset in range(opts.days):
        day = first + timedelta(days=offset)
        day_start = datetime.combine(day, time(), UTC)
        day_end = day_start + timedelta(days=1)
        volume = (
            opts.clicks_per_day * (0.6 if day.weekday() >= 5 else 1.0) * rng.uniform(0.85, 1.15)
        )

        weights: list[float] = []
        for link in links:
            alive = link.created_at < day_end and (
                link.deleted_at is None or link.deleted_at > day_start
            )
            w = link.weight if alive else 0.0
            if link is launch and alive:
                age = (day - launch.created_at.date()).days
                w += 3.0 * math.exp(-age / 2.5)  # launch day: half the traffic, then it fades
            weights.append(w)
        if sum(weights) == 0:
            continue

        n = round(volume)
        times: list[tuple[int, str]] = []
        picks = rng.choices(links, weights, k=n)
        hours = rng.choices(range(24), HOUR_WEIGHTS, k=n)
        for link, hour in zip(picks, hours, strict=True):
            at = day_start + timedelta(hours=hour, milliseconds=rng.randrange(3_600_000))
            # A click can't happen before the link exists or after it's deleted.
            if at < link.created_at or (link.deleted_at is not None and at >= link.deleted_at):
                continue
            times.append((_ms(at), link.slug))
            link.clicks += 1
        times.sort()

        # Redis Stream IDs: <milliseconds>-<sequence within that millisecond>.
        event_ids: list[str] = []
        seq_in_ms: dict[int, int] = defaultdict(int)
        for ms, _slug in times:
            event_ids.append(f"{ms}-{seq_in_ms[ms]}")
            seq_in_ms[ms] += 1
        part = lk.partition_dir(lake, "clicks", day)
        lk.write_parquet_atomic(_clicks_table(event_ids, times), part / "generated.parquet")
        total += len(times)

        # Redelivered copies land in a second file, as a restarted consumer would write them.
        dup_idx = [i for i in range(len(times)) if rng.random() < opts.duplicate_rate]
        if dup_idx:
            lk.write_parquet_atomic(
                _clicks_table([event_ids[i] for i in dup_idx], [times[i] for i in dup_idx]),
                part / "generated-redelivered.parquet",
            )
            duplicates += len(dup_idx)

    _write_snapshots(lake, end, extracted_at, owners, links)
    _write_audit(lake, owners, links)
    (lake / lk.GENERATED_MARKER).write_text(
        json.dumps(
            {
                "seed": opts.seed,
                "days": opts.days,
                "end": end.isoformat(),
                "links": opts.links,
                "clicks_per_day": opts.clicks_per_day,
                "duplicate_rate": opts.duplicate_rate,
            },
            indent=2,
        )
        + "\n"
    )
    return Summary(
        days=opts.days,
        links=len(links),
        deleted_links=sum(1 for x in links if x.deleted_at is not None),
        clicks=total,
        duplicates=duplicates,
        first_day=first,
        last_day=end,
    )


def _prepare(lake: Path) -> None:
    """Refuse to overwrite real data; clear a lake this generator wrote before."""
    if lake.exists():
        marker = lake / lk.GENERATED_MARKER
        has_data = any(lk.has_files(lake, t) for t in lk.TABLES)
        if has_data and not marker.exists():
            raise RuntimeError(
                f"{lake} holds data that wasn't generated (no {lk.GENERATED_MARKER}); "
                "pick another folder with --lake"
            )
        for t in lk.TABLES:  # same seed, fewer days: no stale partitions left behind
            shutil.rmtree(lake / t, ignore_errors=True)
    lake.mkdir(parents=True, exist_ok=True)


def _clicks_table(event_ids: list[str], times: list[tuple[int, str]]) -> pa.Table:
    return pa.table(
        {
            "event_id": pa.array(event_ids, pa.string()),
            "slug": pa.array([s for _, s in times], pa.string()),
            "clicked_at": pa.array([ms for ms, _ in times], pa.int64()).cast(lk.UTC_MS),
        },
        schema=lk.CLICKS_SCHEMA,
    )


def _write_snapshots(
    lake: Path,
    end: date,
    extracted_at: datetime,
    owners: list[tuple[int, str, datetime]],
    links: list[_Link],
) -> None:
    live = sorted((x for x in links if x.deleted_at is None), key=lambda x: x.slug)
    links_tbl = pa.table(
        {
            "slug": [x.slug for x in live],
            "url": [x.url for x in live],
            "owner_id": [x.owner_id for x in live],
            "clicks": [x.clicks for x in live],
            "created_at": [x.created_at for x in live],
            "extracted_at": [extracted_at] * len(live),
        },
        schema=lk.LINKS_SCHEMA,
    )
    lk.write_parquet_atomic(links_tbl, lk.partition_dir(lake, "links", end) / "snapshot.parquet")
    owners_tbl = pa.table(
        {
            "owner_id": [o[0] for o in owners],
            "name": [o[1] for o in owners],
            "created_at": [o[2] for o in owners],
            "revoked_at": [None] * len(owners),
            "extracted_at": [extracted_at] * len(owners),
        },
        schema=lk.OWNERS_SCHEMA,
    )
    lk.write_parquet_atomic(owners_tbl, lk.partition_dir(lake, "owners", end) / "snapshot.parquet")


def _write_audit(lake: Path, owners: list[tuple[int, str, datetime]], links: list[_Link]) -> None:
    events: list[tuple[datetime, int, str, str | None, dict[str, str]]] = []
    for oid, name, created in owners:
        events.append((created, oid, "key.created", None, {"name": name}))
    for x in links:
        events.append((x.created_at, x.owner_id, "link.created", x.slug, {"url": x.url}))
        if x.deleted_at is not None:
            events.append((x.deleted_at, x.owner_id, "link.deleted", x.slug, {"url": x.url}))
    events.sort(key=lambda e: e[0])
    rows = [(i + 1, *e) for i, e in enumerate(events)]  # ids follow time, like a sequence
    by_day: dict[date, list[tuple[int, datetime, int, str, str | None, dict[str, str]]]] = (
        defaultdict(list)
    )
    for r in rows:
        by_day[r[1].date()].append(r)
    for day, day_rows in by_day.items():
        tbl = pa.table(
            {
                "id": [r[0] for r in day_rows],
                "at": [r[1] for r in day_rows],
                "owner_id": [r[2] for r in day_rows],
                "action": [r[3] for r in day_rows],
                "slug": [r[4] for r in day_rows],
                "request_id": [None] * len(day_rows),
                "detail": [json.dumps(r[5]) for r in day_rows],
            },
            schema=lk.AUDIT_SCHEMA,
        )
        name = f"part-{day_rows[0][0]:012d}-{day_rows[-1][0]:012d}.parquet"
        lk.write_parquet_atomic(tbl, lk.partition_dir(lake, "audit_events", day) / name)
