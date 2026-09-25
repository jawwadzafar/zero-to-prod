"""Measure how fast a local model really is (chapter 14.11).

For each model: time to first token (what a person waiting feels), generation
speed in tokens per second (from Ollama's own counters), and total throughput
when several requests arrive at once.

    uv run python -m snipai.bench qwen2.5:0.5b qwen2.5:1.5b
"""

from __future__ import annotations

import json
import os
import statistics
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass

import httpx

PROMPT = "Explain in three sentences what a URL shortener does and why people use one."


@dataclass
class Sample:
    first_token_s: float  # from sending the request to the first piece of text
    total_s: float
    output_tokens: int
    prompt_tokens: int
    gen_tokens_per_s: float  # Ollama's measured decode speed
    prompt_tokens_per_s: float  # how fast the prompt was read


def one_request(http: httpx.Client, model: str, max_tokens: int = 120) -> Sample:
    body = {
        "model": model,
        "prompt": PROMPT,
        "stream": True,
        "options": {"num_predict": max_tokens, "temperature": 0, "seed": 42},
    }
    start = time.perf_counter()
    first = 0.0
    final: dict[str, float] = {}
    with http.stream("POST", "/api/generate", json=body) as res:
        res.raise_for_status()
        for line in res.iter_lines():
            if not line:
                continue
            msg = json.loads(line)
            if not first and msg.get("response"):
                first = time.perf_counter() - start
            if msg.get("done"):
                final = msg
    total = time.perf_counter() - start
    ns = 1e9
    return Sample(
        first_token_s=first,
        total_s=total,
        output_tokens=int(final.get("eval_count", 0)),
        prompt_tokens=int(final.get("prompt_eval_count", 0)),
        gen_tokens_per_s=final.get("eval_count", 0) / max(final.get("eval_duration", 1), 1) * ns,
        prompt_tokens_per_s=final.get("prompt_eval_count", 0)
        / max(final.get("prompt_eval_duration", 1), 1)
        * ns,
    )


def throughput(http: httpx.Client, model: str, parallel: int, requests: int = 8) -> float:
    """Total output tokens per second with `parallel` requests in flight."""
    start = time.perf_counter()
    with ThreadPoolExecutor(parallel) as pool:
        samples = list(pool.map(lambda _: one_request(http, model), range(requests)))
    return sum(s.output_tokens for s in samples) / (time.perf_counter() - start)


def main(argv: list[str] | None = None) -> int:
    models = (argv if argv is not None else sys.argv[1:]) or ["qwen2.5:0.5b"]
    url = os.environ.get("OLLAMA_URL", "http://localhost:11434")
    with httpx.Client(base_url=url, timeout=300.0) as http:
        print(f"prompt: {PROMPT!r}\n")
        header = ("model", "first token", "generate", "read prompt", "1 at a time", "4 at once")
        widths = (16, 12, 14, 15, 14, 12)
        print(
            header[0].ljust(widths[0])
            + "".join(h.rjust(w) for h, w in zip(header[1:], widths[1:], strict=True))
        )
        for model in models:
            one_request(http, model)  # warm-up: loads the model into memory
            samples = [one_request(http, model) for _ in range(3)]
            first = statistics.median(s.first_token_s for s in samples)
            gen = statistics.median(s.gen_tokens_per_s for s in samples)
            read = statistics.median(s.prompt_tokens_per_s for s in samples)
            t1 = throughput(http, model, parallel=1)
            t4 = throughput(http, model, parallel=4)
            print(
                f"{model:<16}{first:>10.2f} s{gen:>10.1f} t/s{read:>11.0f} t/s"
                f"{t1:>10.1f} t/s{t4:>8.1f} t/s"
            )
    return 0


if __name__ == "__main__":
    sys.exit(main())
