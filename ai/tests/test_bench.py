from __future__ import annotations

import json

import httpx

from snipai.bench import one_request


def test_one_request_reads_ollamas_counters() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        lines = [
            {"response": "A URL", "done": False},
            {"response": " shortener...", "done": False},
            {"response": "", "done": True, "eval_count": 50, "eval_duration": 2_000_000_000,
             "prompt_eval_count": 20, "prompt_eval_duration": 100_000_000},
        ]  # fmt: skip
        return httpx.Response(200, content="\n".join(json.dumps(x) for x in lines).encode())

    with httpx.Client(
        base_url="http://ollama.test", transport=httpx.MockTransport(handler)
    ) as http:
        s = one_request(http, "tiny")
    assert s.output_tokens == 50 and s.gen_tokens_per_s == 25.0  # 50 tokens in 2 s
    assert s.prompt_tokens_per_s == 200.0 and s.first_token_s > 0


def test_every_request_has_a_different_prompt() -> None:
    prompts: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        prompts.append(json.loads(request.content)["prompt"])
        done = {"response": "ok", "done": True, "eval_count": 1, "eval_duration": 1}
        return httpx.Response(200, content=json.dumps(done).encode())

    with httpx.Client(
        base_url="http://ollama.test", transport=httpx.MockTransport(handler)
    ) as http:
        one_request(http, "tiny")
        one_request(http, "tiny", context="Some sources. ")
    assert len(set(prompts)) == 2  # no identical prompts for the server to reuse
    assert "Some sources. " in prompts[1] and prompts[1].endswith("use one.")
