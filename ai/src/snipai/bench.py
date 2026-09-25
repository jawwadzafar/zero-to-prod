"""Measure how fast a local model really is (chapter 14.11).

For each model: time to first token (what a person waiting feels) for a short
and a long prompt, generation speed in tokens per second (from Ollama's own
counters), and total throughput when several requests arrive at once.

Every request starts with a random tag, so the server can't reuse the
previous request's work on an identical prompt (servers cache that, and it
would make the first token look instant).

    uv run python -m snipai.bench qwen2.5:0.5b qwen2.5:1.5b
"""

from __future__ import annotations

import json
import os
import statistics
import sys
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass

import httpx

PROMPT = "Explain in three sentences what a URL shortener does and why people use one."

# About 1,500 tokens of real text, like a RAG prompt with a few sources (chapter 14.8).
LONG_CONTEXT = (
    "A Redis stream is an append-only log: each entry gets an ID based on the time it was "
    "added, and entries stay in order. snip's redirect handler appends one click event per "
    "redirect, and a worker reads them in batches, adds them up, and writes the totals to "
    "Postgres, then acknowledges them so they aren't processed twice. "
) * 20


@dataclass
class Sample:
    first_token_s: float  # from sending the request to the first piece of text
    total_s: float
    output_tokens: int
    prompt_tokens: int
    gen_tokens_per_s: float  # Ollama's measured decode speed
    prompt_tokens_per_s: float  # how fast the prompt was read


def one_request(http: httpx.Client, model: str, max_tokens: int = 120, context: str = "") -> Sample:
    # A random tag at the very start: no two prompts share a first token, even
    # across separate runs against the same server, so none can reuse the
    # server's cached work from an earlier request.
    prompt = f"Request {uuid.uuid4().hex[:12]}. {context}{PROMPT}"
    body = {
        "model": model,
        "prompt": prompt,
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
    parallel = os.environ.get("OLLAMA_NUM_PARALLEL", "(server default)")
    with httpx.Client(base_url=url, timeout=300.0) as http:
        print(f"prompt: {PROMPT!r}; server parallel setting: {parallel}\n")
        header = ("model", "first token", "long prompt", "generate", "read prompt", "1 at a time",
                  "4 at once")  # fmt: skip
        widths = (14, 12, 13, 12, 13, 13, 11)
        print(
            header[0].ljust(widths[0])
            + "".join(h.rjust(w) for h, w in zip(header[1:], widths[1:], strict=True))
        )
        tokens = ""
        for model in models:
            one_request(http, model)  # warm-up: loads the model into memory
            short = [one_request(http, model) for _ in range(3)]
            long = [one_request(http, model, context=LONG_CONTEXT) for _ in range(3)]
            first = statistics.median(s.first_token_s for s in short)
            first_long = statistics.median(s.first_token_s for s in long)
            gen = statistics.median(s.gen_tokens_per_s for s in short)
            read = statistics.median(s.prompt_tokens_per_s for s in long)
            t1 = throughput(http, model, parallel=1)
            t4 = throughput(http, model, parallel=4)
            print(
                f"{model:<14}{first:>10.2f} s{first_long:>11.2f} s{gen:>8.1f} t/s"
                f"{read:>9.0f} t/s{t1:>9.1f} t/s{t4:>7.1f} t/s"
            )
            tokens += (
                f"  {model}: short prompt {short[0].prompt_tokens} tokens, long prompt "
                f"{long[0].prompt_tokens} tokens, answers {short[0].output_tokens} tokens\n"
            )
        print("\n" + tokens, end="")
    return 0


if __name__ == "__main__":
    sys.exit(main())
