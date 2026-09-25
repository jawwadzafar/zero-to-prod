"""Structured output and tool use (chapter 14.6), offline."""

from __future__ import annotations

import json
from typing import Any

import anthropic
import httpx
import httpx2
import pytest

from snipai.ask import ask, run_tool, snip_tools
from snipai.client import SnipClient
from snipai.llm import (
    AnthropicLLM,
    Completion,
    FakeLLM,
    Message,
    OllamaLLM,
    ToolCall,
    ToolSpec,
    Turn,
)
from snipai.pages import Page
from snipai.tagger import SCHEMA, LinkTags, TaggingError, tag


def link(slug: str, url: str, clicks: int) -> dict[str, Any]:
    return {"slug": slug, "url": url, "clicks": clicks, "created_at": "2026-09-24T12:00:00Z",
            "short_url": f"http://snip.test/{slug}"}  # fmt: skip


LINKS = [link("go", "https://go.dev", 12), link("py", "https://python.org", 40)]


def fake_snip(request: httpx.Request) -> httpx.Response:
    if request.url.path == "/api/links":
        return httpx.Response(200, json={"links": LINKS})
    if request.url.path == "/api/links/py":
        return httpx.Response(200, json=LINKS[1])
    return httpx.Response(404, json={"error": {"code": "not_found", "message": "no such link"}})


def tools() -> dict[str, Any]:
    return snip_tools(SnipClient("http://snip.test", "k", transport=httpx.MockTransport(fake_snip)))


# --- structured output -------------------------------------------------------


def test_schema_is_strict_for_structured_output_apis() -> None:
    assert SCHEMA["additionalProperties"] is False
    assert set(SCHEMA["required"]) == {"category", "tags", "language", "needs_login"}


class Replies:
    """A fake model that gives scripted answers, in order."""

    name, model = "scripted", "scripted"

    def __init__(self, *texts: str) -> None:
        self.texts = list(texts)
        self.prompts: list[str] = []

    def complete(self, prompt: str, **kw: Any) -> Completion:
        self.prompts.append(prompt)
        return Completion(self.texts.pop(0), "scripted", 1, 1, "end_turn", 0.0)


PAGE = Page("https://go.dev", "Go", "Go is a programming language.")
GOOD = json.dumps(
    {"category": "software", "tags": ["Golang", "golang", "programming"],
     "language": "EN", "needs_login": False}
)  # fmt: skip


def test_valid_answer_is_parsed_and_cleaned() -> None:
    tags, calls = tag(PAGE, Replies(GOOD))  # type: ignore[arg-type]
    assert tags == LinkTags(
        category="software", tags=["golang", "programming"], language="en", needs_login=False
    )
    assert len(calls) == 1


def test_invalid_answer_is_retried_with_the_error() -> None:
    bad = '{"category": "software", "tags": [], "language": "English", "needs_login": false}'
    model = Replies(bad, GOOD)
    tags, calls = tag(PAGE, model)  # type: ignore[arg-type]
    assert len(calls) == 2 and tags.language == "en"
    assert "give between 1 and 5 tags" in model.prompts[1]  # the error was fed back


def test_gives_up_after_the_retry() -> None:
    with pytest.raises(TaggingError):
        tag(PAGE, Replies("not json", "still not json"))  # type: ignore[arg-type]


def test_fake_model_answers_in_the_schema_shape() -> None:
    # Valid JSON of the right shape, but meaningless: the validator rejects it.
    c = FakeLLM().complete("classify", schema=SCHEMA)
    data = json.loads(c.text)
    assert set(data) == {"category", "tags", "language", "needs_login"}
    with pytest.raises(TaggingError):
        tag(PAGE, FakeLLM())


# --- the tool loop -----------------------------------------------------------


def test_run_tool_validates_everything() -> None:
    t = tools()
    assert "no tool called" in run_tool(t, "delete_everything", {})
    assert "invalid arguments" in run_tool(t, "get_link", {"slug": 7, "extra": 1})
    assert "not_found" in run_tool(t, "get_link", {"slug": "nope"})
    assert json.loads(run_tool(t, "get_link", {"slug": "py"}))["clicks"] == 40


class Script:
    """A fake model following a script of turns."""

    name, model = "script", "script"

    def __init__(self, *turns: Turn) -> None:
        self.turns = list(turns)
        self.seen: list[list[Message]] = []

    def chat(self, messages: list[Message], tools: list[ToolSpec], **kw: Any) -> Turn:
        self.seen.append(list(messages))
        return self.turns.pop(0)


def turn(text: str = "", *calls: ToolCall) -> Turn:
    return Turn(text, list(calls), Completion(text, "script", 10, 5, "end_turn", 0.0))


def test_loop_runs_the_requested_tool_and_returns_the_answer() -> None:
    model = Script(
        turn("", ToolCall("c1", "list_links", {"limit": 10})), turn("py has the most clicks (40).")
    )
    answer = ask("Which link has the most clicks?", model, tools())  # type: ignore[arg-type]
    assert answer.text == "py has the most clicks (40)."
    assert answer.steps[0].startswith('list_links({"limit": 10})')
    tool_msg = model.seen[1][-1]  # the second turn saw the tool's result
    assert tool_msg.role == "tool" and tool_msg.call_id == "c1" and '"clicks": 40' in tool_msg.text


def test_loop_stops_a_model_that_never_answers() -> None:
    forever = [turn("", ToolCall(f"c{i}", "list_links", {})) for i in range(10)]
    answer = ask("?", Script(*forever), tools(), max_steps=3)  # type: ignore[arg-type]
    assert "stopped after 3 steps" in answer.text and len(answer.steps) == 3


def test_fake_model_uses_a_tool_then_answers() -> None:
    answer = ask("Which link has the most clicks?", FakeLLM(), tools())
    assert answer.steps and "list_links" in answer.steps[0]
    assert '"clicks": 40' in answer.text


# --- the providers' wire formats ---------------------------------------------


def test_anthropic_tool_use_round_trip() -> None:
    bodies: list[dict[str, Any]] = []

    def handler(request: httpx2.Request) -> httpx2.Response:
        body = json.loads(request.content)
        bodies.append(body)
        if len(bodies) == 1:
            content: list[dict[str, Any]] = [
                {"type": "tool_use", "id": "toolu_1", "name": "list_links", "input": {"limit": 5}}
            ]
            stop = "tool_use"
        else:
            content, stop = [{"type": "text", "text": "py has 40 clicks."}], "end_turn"
        return httpx2.Response(
            200,
            json={
                "id": "m", "type": "message", "role": "assistant", "model": "claude-haiku-4-5",
                "content": content, "stop_reason": stop, "stop_sequence": None,
                "usage": {"input_tokens": 50, "output_tokens": 10},
            },
        )  # fmt: skip

    client = anthropic.Anthropic(
        api_key="k",
        max_retries=0,
        http_client=anthropic.DefaultHttpxClient(transport=httpx2.MockTransport(handler)),  # type: ignore[arg-type]
    )
    answer = ask("Most clicks?", AnthropicLLM(client=client), tools())
    assert answer.text == "py has 40 clicks."
    first, second = bodies
    assert first["tools"][0]["name"] == "list_links" and "input_schema" in first["tools"][0]
    # The tool call and its result are echoed back in Anthropic's block format.
    assert second["messages"][1]["content"][0]["type"] == "tool_use"
    result = second["messages"][2]
    assert result["role"] == "user" and result["content"][0]["type"] == "tool_result"
    assert result["content"][0]["tool_use_id"] == "toolu_1"


def test_ollama_tool_calls_and_structured_output() -> None:
    bodies: list[dict[str, Any]] = []

    def handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        bodies.append(body)
        if "format" in body:  # structured output request
            return httpx.Response(200, json={"message": {"content": GOOD}, "done_reason": "stop"})
        if len(bodies) == 1:
            call = {"function": {"name": "get_link", "arguments": {"slug": "py"}}}
            return httpx.Response(200, json={"message": {"content": "", "tool_calls": [call]}})
        return httpx.Response(200, json={"message": {"content": "py: 40 clicks."}})

    llm = OllamaLLM(transport=httpx.MockTransport(handler))
    answer = ask("How many clicks does py have?", llm, tools())
    assert answer.text == "py: 40 clicks."
    assert bodies[0]["tools"][0]["type"] == "function"
    assert bodies[1]["messages"][-1] == {
        "role": "tool",
        "content": answer.steps[0].split("-> ")[1],
        "tool_name": "get_link",
    }
    tags, _ = tag(PAGE, llm)
    assert tags.category == "software" and bodies[-1]["format"] == SCHEMA


def test_empty_answers_are_reported_not_passed_on() -> None:
    answer = ask("Delete all my links.", Script(turn("")), tools())  # type: ignore[arg-type]
    assert answer.text == "(the model gave no answer)"
