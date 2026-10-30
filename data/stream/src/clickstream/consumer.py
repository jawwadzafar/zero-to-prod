"""Read snip's click stream as the `analytics` consumer group and land it in the lake.

snip's own workers read the same stream in their group, `workers`, to count clicks
in Postgres (snip/internal/clicks/stream.go). A consumer group keeps its own place
in the stream, so this group sees every event too, without taking any from them.

Each call to process_once handles one batch:

1. our own pending events (delivered to us before, never acknowledged: we crashed);
2. else other analytics consumers' pending events, idle too long (XAUTOCLAIM);
3. else new events, collected until the batch is full or the oldest has waited
   flush_seconds;

then writes them to Parquet (lake.write_batch) and only after that acknowledges
them (XACK). A crash anywhere before the XACK means the batch is delivered again
and rewritten to the same file names: at-least-once delivery, an idempotent sink,
effectively-once output.
"""

from __future__ import annotations

import json
import logging
import os
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import redis

from clickstream import lake
from clickstream.config import Config
from clickstream.events import BadEvent, Click, id_key, parse

STREAM = "snip:clicks"  # snip's StreamKey
GROUP = "analytics"  # ours; snip's workers are in "workers"

log = logging.getLogger("clickstream")

Entry = tuple[str, dict[str, str] | None]  # (ID, fields); fields None = trimmed away


@dataclass(frozen=True)
class Gap:
    """Events the stream trimmed (MAXLEN) before this group read them: gone for good."""

    after_id: str  # the last event this group had received
    first_id: str  # the oldest event still in the stream
    lost: int | None  # how many were trimmed unread; None when Redis can't tell

    def describe(self) -> str:
        n = "an unknown number of" if self.lost is None else str(self.lost)
        return (
            f"{n} click events were trimmed from {STREAM} before the {GROUP} group "
            f"read them (after {self.after_id}, before {self.first_id})"
        )


@dataclass
class BatchResult:
    source: str = ""  # "pending", "claimed" or "new"
    events: int = 0  # entries handled, all acknowledged
    written: int = 0  # clicks written to Parquet
    bad: int = 0  # entries that can never be clicks (poison), skipped
    trimmed: int = 0  # entries trimmed from the stream while pending with us
    files: list[Path] = field(default_factory=list)


class Consumer:
    def __init__(
        self,
        r: redis.Redis,
        cfg: Config,
        should_stop: Callable[[], bool] = lambda: False,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self.r = r
        self.cfg = cfg
        self.should_stop = should_stop
        self.clock = clock

    def ensure_group(self) -> None:
        """Create the group at the start of the stream ("0"): backfill whatever is still there."""
        try:
            self.r.xgroup_create(STREAM, GROUP, id="0", mkstream=True)
        except redis.ResponseError as err:
            if "BUSYGROUP" not in str(err):  # the group already exists: that's fine
                raise

    def process_once(self) -> BatchResult:
        res = BatchResult(source="pending")
        entries = self._read("0", self.cfg.batch_size, block_ms=None)
        if not entries:
            res.source = "claimed"
            entries, res.trimmed = self._autoclaim()
        if not entries:
            res.source = "new"
            gap = self.check_gap()
            if gap is not None:
                self.report_gap(gap)
            entries = self._collect_new()
        if not entries:
            self._warn_trimmed(res)
            return res

        clicks: list[Click] = []
        for entry_id, fields in entries:
            if fields is None:  # delivered to us, then trimmed before we acknowledged it
                res.trimmed += 1
                continue
            try:
                clicks.append(parse(entry_id, fields))
            except BadEvent as err:
                res.bad += 1  # a poison message: retrying can't fix it, so record and move on
                log.warning("skipping bad event: %s", err)

        res.files = lake.write_batch(self.cfg.lake_dir, clicks) if clicks else []
        res.written = len(clicks)
        if self.cfg.crash_after_write:  # lab: the worst moment to die
            log.error("CLICKSTREAM_CRASH_AFTER_WRITE=1: exiting before XACK")
            os._exit(3)
        # Only now: every event is either in a durable file or deliberately skipped.
        self.r.xack(STREAM, GROUP, *[entry_id for entry_id, _ in entries])
        res.events = len(entries)
        self._warn_trimmed(res)
        return res

    def _warn_trimmed(self, res: BatchResult) -> None:
        if res.trimmed:
            log.warning(
                "%d pending events were trimmed from the stream before they were "
                "acknowledged; unless an earlier run wrote them, they are lost",
                res.trimmed,
            )

    def check_gap(self) -> Gap | None:
        """Has the stream trimmed events this group never read?

        MAXLEN trimming removes the oldest entries. If the oldest entry still in
        the stream is newer than the last one we received, everything between
        was trimmed unread. Redis counts every entry ever added (entries-added)
        and every entry the group has read (entries-read), so the difference,
        minus what is still in the stream, is how many we lost.
        """
        stream = self._xinfo_stream()
        group = self._xinfo_group()
        if stream is None or group is None:
            return None
        last = str(group["last-delivered-id"])
        length = int(stream["length"])
        added = int(stream["entries-added"])
        first = _first_id(stream)
        if first is not None and id_key(last) >= id_key(first):
            return None  # nothing between our place and the stream's start
        read: int | None = group.get("entries-read")
        if read is None and last == "0-0":
            read = 0  # a new group has read nothing yet
        first_label = first or "(the stream is empty)"
        if read is None:
            return Gap(last, first_label, None)
        lost = added - int(read) - length  # every entry after `last` is unread
        return Gap(last, first_label, lost) if lost > 0 else None

    def report_gap(self, gap: Gap) -> None:
        """Log the gap and keep a durable record next to the data, for whoever asks why."""
        log.warning("DATA GAP: %s", gap.describe())
        record = {
            "detected_at": datetime.now(UTC).isoformat(timespec="milliseconds"),
            "stream": STREAM,
            "group": GROUP,
            "after_id": gap.after_id,
            "first_id": gap.first_id,
            "lost": gap.lost,
        }
        path = self.cfg.lake_dir / "_gaps" / "clicks.jsonl"
        path.parent.mkdir(parents=True, exist_ok=True)
        with open(path, "a", encoding="utf-8") as f:
            f.write(json.dumps(record) + "\n")
            f.flush()
            os.fsync(f.fileno())

    def lag(self) -> int | None:
        """Events in the stream not yet delivered to this group (None if Redis can't tell)."""
        group = self._xinfo_group()
        if group is None or group.get("lag") is None:
            return None
        return int(group["lag"])

    # --- reading --------------------------------------------------------------

    def _read(self, start: str, count: int, block_ms: int | None) -> list[Entry]:
        """XREADGROUP: start "0" is our own pending entries, ">" is never-delivered ones."""
        reply: Any = self.r.xreadgroup(
            GROUP, self.cfg.consumer, {STREAM: start}, count=count, block=block_ms
        )
        entries: list[Entry] = []
        for _stream, msgs in reply or []:
            for entry_id, fields in msgs:
                entries.append((str(entry_id), dict(fields) if fields else None))
        return entries

    def _autoclaim(self) -> tuple[list[Entry], int]:
        """Take over entries another analytics consumer received and never acknowledged.

        Returns the entries and how many pending entries Redis found already
        trimmed from the stream (it drops those from the pending list itself).
        """
        reply: Any = self.r.xautoclaim(
            STREAM,
            GROUP,
            self.cfg.consumer,
            min_idle_time=int(self.cfg.claim_idle_seconds * 1000),
            start_id="0-0",
            count=self.cfg.batch_size,
        )
        msgs = reply[1] if reply else []
        deleted = reply[2] if reply and len(reply) > 2 else []
        entries = [(str(i), dict(f) if f else None) for i, f in msgs]
        return entries, len(deleted)

    def _collect_new(self) -> list[Entry]:
        """New events until the batch is full or the oldest has waited flush_seconds.

        Returns [] if nothing arrives within a second, so the caller can check
        whether it has been asked to stop.
        """
        out: list[Entry] = []
        deadline: float | None = None
        while len(out) < self.cfg.batch_size:
            if deadline is None:
                block_ms = 1000
            else:
                remaining = deadline - self.clock()
                if remaining <= 0:
                    break
                block_ms = max(1, min(1000, int(remaining * 1000)))
            got = self._read(">", self.cfg.batch_size - len(out), block_ms)
            if got:
                if deadline is None:  # the batch's clock starts with its first event
                    deadline = self.clock() + self.cfg.flush_seconds
                out.extend(got)
            elif deadline is None:
                return out
            if self.should_stop():
                break  # write what we have, then exit
        return out

    def _xinfo_stream(self) -> dict[str, Any] | None:
        try:
            info: dict[str, Any] = self.r.xinfo_stream(STREAM)
        except redis.ResponseError:  # no such stream yet
            return None
        return info

    def _xinfo_group(self) -> dict[str, Any] | None:
        try:
            groups: list[dict[str, Any]] = self.r.xinfo_groups(STREAM)
        except redis.ResponseError:
            return None
        return next((g for g in groups if g["name"] == GROUP), None)


def _first_id(stream: dict[str, Any]) -> str | None:
    """The oldest entry ID still in the stream, or None if it is empty."""
    if int(stream["length"]) == 0:
        return None
    first = stream.get("first-entry")
    if first:
        return str(first[0])
    recorded = stream.get("recorded-first-entry-id")
    return str(recorded) if recorded else None
