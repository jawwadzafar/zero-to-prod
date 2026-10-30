"""One click event, parsed from a stream entry.

snip's API appends each click with XADD snip:clicks * slug <slug> at <unix ms>
(snip/internal/clicks/stream.go). The entry's ID, which Redis assigns, is unique
in the stream, so it becomes event_id: the key every later step deduplicates on.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime


@dataclass(frozen=True)
class Click:
    event_id: str  # the stream entry ID, e.g. "1793282586265-0"
    slug: str
    clicked_at: datetime  # event time: when the click happened, in UTC


class BadEvent(ValueError):
    """An entry that can never become a Click (a poison message)."""


def id_millis(entry_id: str) -> int:
    """The milliseconds part of a stream ID: when Redis appended the entry."""
    return int(entry_id.split("-", 1)[0])


def parse(entry_id: str, fields: dict[str, str]) -> Click:
    slug = fields.get("slug", "")
    if not slug:
        raise BadEvent(f"{entry_id}: no slug")
    raw_at = fields.get("at")
    try:
        # The event time snip recorded. If it's missing, the ID's own timestamp
        # is the next best thing: the moment Redis stored the event.
        millis = int(raw_at) if raw_at is not None else id_millis(entry_id)
    except ValueError as err:
        raise BadEvent(f"{entry_id}: at={raw_at!r} is not unix milliseconds") from err
    if millis < 0:
        raise BadEvent(f"{entry_id}: at={raw_at!r} is before 1970")
    # Integer arithmetic, so 1793282586265 ms stays exactly .265 s (a float can round).
    try:
        seconds = datetime.fromtimestamp(millis // 1000, tz=UTC)
        at = seconds.replace(microsecond=millis % 1000 * 1000)
    except (OverflowError, OSError, ValueError) as err:  # e.g. a year past 9999
        raise BadEvent(f"{entry_id}: at={raw_at!r} is out of range") from err
    return Click(entry_id, slug, at)


def id_key(entry_id: str) -> tuple[int, int]:
    """Sort key for stream IDs: compare as numbers, not text ("9-0" < "10-0")."""
    ms, _, seq = entry_id.partition("-")
    return int(ms), int(seq or 0)
