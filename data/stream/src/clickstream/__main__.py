"""Run the analytics consumer until SIGINT or SIGTERM: python -m clickstream."""

from __future__ import annotations

import logging
import signal
import sys
import threading
from types import FrameType

import redis

from clickstream import lake
from clickstream.config import Config
from clickstream.consumer import GROUP, STREAM, Consumer

log = logging.getLogger("clickstream")


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
        stream=sys.stderr,
    )
    cfg = Config.from_env()
    stop = threading.Event()

    def on_signal(signum: int, _frame: FrameType | None) -> None:
        log.info("signal %d: finishing the current batch, then stopping", signum)
        stop.set()

    signal.signal(signal.SIGTERM, on_signal)  # docker stop, Kubernetes
    signal.signal(signal.SIGINT, on_signal)  # Ctrl-C

    r = redis.Redis(
        host=cfg.redis_host,
        port=cfg.redis_port,
        password=cfg.redis_password or None,
        decode_responses=True,
        health_check_interval=30,
    )
    removed = lake.remove_leftovers(cfg.lake_dir)
    if removed:
        log.info("removed %d unfinished temporary files from an earlier crash", removed)

    c = Consumer(r, cfg, should_stop=stop.is_set)
    backoff = 0.5
    log.info(
        "reading %s as %s/%s into %s (batch %d, flush %.0fs)",
        STREAM, GROUP, cfg.consumer, lake.clicks_dir(cfg.lake_dir),
        cfg.batch_size, cfg.flush_seconds,
    )  # fmt: skip
    group_ready = False
    while not stop.is_set():
        try:
            if not group_ready:
                c.ensure_group()
                group_ready = True
            res = c.process_once()
        except redis.ConnectionError as err:
            log.warning("redis unavailable (%s); retrying in %.1fs", err, backoff)
            stop.wait(backoff)
            backoff = min(backoff * 2, 30.0)  # exponential backoff (chapter 8.1)
            continue
        backoff = 0.5
        if res.events:
            log.info(
                "%s: %d events, %d written, %d bad, %d trimmed -> %s",
                res.source, res.events, res.written, res.bad, res.trimmed,
                ", ".join(str(p) for p in res.files) or "no file",
            )  # fmt: skip
    log.info("stopped")


if __name__ == "__main__":
    main()
