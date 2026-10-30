"""Settings from environment variables, the same way snip reads its own (chapter 11.5)."""

from __future__ import annotations

import os
import socket
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Config:
    redis_addr: str = "localhost:6379"  # host:port, like snip's SNIP_REDIS_ADDR
    redis_password: str = ""  # SNIP_REDIS_PASSWORD; empty means no AUTH
    lake_dir: Path = Path("../lake")  # the lake's root; files go under <lake_dir>/clicks/
    consumer: str = "analytics-1"  # this process's name inside the group
    batch_size: int = 1000  # write a file once this many events are waiting...
    flush_seconds: float = 10.0  # ...or once the oldest waiting event is this old
    claim_idle_seconds: float = 60.0  # take over another consumer's events after this long
    crash_after_write: bool = False  # lab only: die after writing a file, before XACK

    @property
    def redis_host(self) -> str:
        return self.redis_addr.rsplit(":", 1)[0]

    @property
    def redis_port(self) -> int:
        _, _, port = self.redis_addr.rpartition(":")
        return int(port) if port else 6379

    @classmethod
    def from_env(cls, env: dict[str, str] | None = None) -> Config:
        e = dict(os.environ) if env is None else env
        return cls(
            redis_addr=e.get("SNIP_REDIS_ADDR", cls.redis_addr),
            redis_password=e.get("SNIP_REDIS_PASSWORD", ""),
            lake_dir=Path(e.get("LAKE_DIR", str(cls.lake_dir))),
            consumer=e.get("CLICKSTREAM_CONSUMER") or socket.gethostname(),
            batch_size=int(e.get("CLICKSTREAM_BATCH_SIZE", cls.batch_size)),
            flush_seconds=float(e.get("CLICKSTREAM_FLUSH_SECONDS", cls.flush_seconds)),
            claim_idle_seconds=float(
                e.get("CLICKSTREAM_CLAIM_IDLE_SECONDS", cls.claim_idle_seconds)
            ),
            crash_after_write=e.get("CLICKSTREAM_CRASH_AFTER_WRITE", "") == "1",
        )
