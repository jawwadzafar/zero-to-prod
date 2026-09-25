"""The model providers, tested without network access or API keys."""

from __future__ import annotations

import json

import anthropic
import httpx
import httpx2

from snipai.llm import AnthropicLLM, FakeLLM, OllamaLLM, estimate_cost


def test_fake_llm_extracts_the_first_sentence_after_text() -> None:
    c = FakeLLM().complete("Describe this.\nTEXT: Go is a language. It is fast.")
    assert c.text == "Go is a language."
    assert c.stop_reason == "end_turn" and c.cost_usd() == 0.0
    assert "".join(FakeLLM().stream("TEXT: One two three.")) == "One two three."


def test_fake_llm_reports_truncation() -> None:
    c = FakeLLM().complete("TEXT: one two three four five.", max_tokens=2)
    assert c.text == "one two" and c.truncated


def test_cost_estimate() -> None:
    # 1M input tokens at $1 + 1M output tokens at $5
    assert estimate_cost("claude-haiku-4-5", 1_000_000, 1_000_000) == 6.0
    assert estimate_cost("some-local-model", 5000, 5000) == 0.0


def fake_anthropic(handler: object) -> anthropic.Anthropic:
    return anthropic.Anthropic(
        api_key="test-key",
        max_retries=0,
        http_client=anthropic.DefaultHttpxClient(transport=httpx2.MockTransport(handler)),  # type: ignore[arg-type]
    )


def test_anthropic_request_and_response() -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx2.Request) -> httpx2.Response:
        seen["path"] = request.url.path
        seen["key"] = request.headers.get("x-api-key")
        seen["body"] = json.loads(request.content)
        return httpx2.Response(
            200,
            json={
                "id": "msg_test",
                "type": "message",
                "role": "assistant",
                "model": "claude-haiku-4-5",
                "content": [{"type": "text", "text": "The Go website."}],
                "stop_reason": "end_turn",
                "stop_sequence": None,
                "usage": {"input_tokens": 120, "output_tokens": 6},
            },
        )

    llm = AnthropicLLM("claude-haiku-4-5", client=fake_anthropic(handler))
    c = llm.complete("Describe go.dev", system="Be brief.", max_tokens=50)
    assert seen["path"] == "/v1/messages" and seen["key"] == "test-key"
    body = seen["body"]
    assert isinstance(body, dict)
    assert body["model"] == "claude-haiku-4-5" and body["max_tokens"] == 50
    assert body["system"] == "Be brief."
    assert body["messages"] == [{"role": "user", "content": "Describe go.dev"}]
    assert (c.text, c.input_tokens, c.output_tokens) == ("The Go website.", 120, 6)
    assert c.cost_usd() == (120 * 1.0 + 6 * 5.0) / 1_000_000


def sse(events: list[dict[str, object]]) -> bytes:
    return "".join(f"event: {e['type']}\ndata: {json.dumps(e)}\n\n" for e in events).encode()


def test_anthropic_streaming() -> None:
    def handler(request: httpx2.Request) -> httpx2.Response:
        assert json.loads(request.content)["stream"] is True
        message = {
            "id": "msg_test", "type": "message", "role": "assistant", "model": "claude-haiku-4-5",
            "content": [], "stop_reason": None, "stop_sequence": None,
            "usage": {"input_tokens": 10, "output_tokens": 0},
        }  # fmt: skip

        def delta(text: str) -> dict[str, object]:
            piece = {"type": "text_delta", "text": text}
            return {"type": "content_block_delta", "index": 0, "delta": piece}

        block = {"type": "text", "text": ""}
        stop = {"stop_reason": "end_turn", "stop_sequence": None}
        events: list[dict[str, object]] = [
            {"type": "message_start", "message": message},
            {"type": "content_block_start", "index": 0, "content_block": block},
            delta("The Go"),
            delta(" website."),
            {"type": "content_block_stop", "index": 0},
            {"type": "message_delta", "delta": stop, "usage": {"output_tokens": 4}},
            {"type": "message_stop"},
        ]
        return httpx2.Response(
            200, content=sse(events), headers={"content-type": "text/event-stream"}
        )

    llm = AnthropicLLM("claude-haiku-4-5", client=fake_anthropic(handler))
    assert list(llm.stream("Describe go.dev")) == ["The Go", " website."]


def test_anthropic_errors_are_typed() -> None:
    def handler(request: httpx2.Request) -> httpx2.Response:
        error = {"type": "rate_limit_error", "message": "slow down"}
        return httpx2.Response(429, json={"type": "error", "error": error})

    llm = AnthropicLLM(client=fake_anthropic(handler))
    try:
        llm.complete("hi")
    except anthropic.RateLimitError as e:
        assert e.status_code == 429
    else:
        raise AssertionError("expected RateLimitError")


def test_ollama_complete_and_stream() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert request.url.path == "/api/chat" and body["model"] == "qwen2.5:0.5b"
        assert body["messages"][0] == {"role": "system", "content": "Be brief."}
        if body["stream"]:
            lines = [{"message": {"content": "The Go"}}, {"message": {"content": " website."}}]
            return httpx.Response(200, content="\n".join(json.dumps(x) for x in lines).encode())
        return httpx.Response(
            200,
            json={
                "message": {"role": "assistant", "content": "The Go website."},
                "done_reason": "stop",
                "prompt_eval_count": 30,
                "eval_count": 5,
            },
        )

    llm = OllamaLLM(transport=httpx.MockTransport(handler))
    c = llm.complete("Describe go.dev", system="Be brief.")
    assert (c.text, c.input_tokens, c.output_tokens, c.cost_usd()) == (
        "The Go website.",
        30,
        5,
        0.0,
    )
    assert "".join(llm.stream("Describe go.dev", system="Be brief.")) == "The Go website."
