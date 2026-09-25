"""The agent loop's safety rails (chapter 14.9), offline."""

from __future__ import annotations

import json
from typing import Any

import httpx

from snipai.agent import run_agent, tidy_tools
from snipai.client import SnipClient
from snipai.llm import Completion, Message, ToolCall, ToolSpec, Turn
from snipai.pages import FetchError


def link(slug: str, url: str) -> dict[str, Any]:
    return {"slug": slug, "url": url, "clicks": 0, "created_at": "2026-09-24T12:00:00Z",
            "short_url": f"http://snip.test/{slug}"}  # fmt: skip


LINKS = {"go": link("go", "https://go.dev/"), "dead": link("dead", "https://gone.example/")}
DELETED: list[str] = []


def fake_snip(request: httpx.Request) -> httpx.Response:
    slug = request.url.path.rsplit("/", 1)[-1]
    if request.method == "GET" and request.url.path == "/api/links":
        return httpx.Response(200, json={"links": list(LINKS.values())})
    if request.method == "GET" and slug in LINKS:
        return httpx.Response(200, json=LINKS[slug])
    if request.method == "DELETE" and slug in LINKS:
        DELETED.append(slug)
        return httpx.Response(204)
    return httpx.Response(404, json={"error": {"code": "not_found", "message": "no such link"}})


def fake_fetch(url: str) -> None:
    if "gone" in url:
        raise FetchError(f"{url} answered 404")


def tools() -> dict[str, Any]:
    DELETED.clear()
    snip = SnipClient("http://snip.test", "k", transport=httpx.MockTransport(fake_snip))
    return tidy_tools(snip, fetch=fake_fetch)


class Script:
    name, model = "script", "script"

    def __init__(self, *turns: Turn) -> None:
        self.turns = list(turns)
        self.results: list[str] = []

    def chat(self, messages: list[Message], tools: list[ToolSpec], **kw: Any) -> Turn:
        if messages[-1].role == "tool":
            self.results.append(messages[-1].text)
        return self.turns.pop(0)


def call(name: str, **args: Any) -> Turn:
    return Turn("", [ToolCall(name, name, args)], Completion("", "s", 100, 10, "tool_use", 0.0))


def say(text: str) -> Turn:
    return Turn(text, [], Completion(text, "s", 100, 10, "end_turn", 0.0))


def script() -> Script:
    return Script(
        call("list_links"),
        call("check_destination", slug="dead"),
        call("delete_link", slug="dead", reason="destination returns 404"),
        say("Deleted 1 broken link: dead."),
    )


def test_dangerous_tool_runs_only_with_approval() -> None:
    run = run_agent("tidy", script(), tools(), system="", confirm=lambda n, a: True)  # type: ignore[arg-type]
    assert DELETED == ["dead"] and run.answer == "Deleted 1 broken link: dead."
    assert [s.kind for s in run.steps] == ["tool", "tool", "tool", "answer"]


def test_declined_action_is_not_run_and_the_model_is_told() -> None:
    model = script()
    run = run_agent("tidy", model, tools(), system="", confirm=lambda n, a: False)  # type: ignore[arg-type]
    assert DELETED == []
    assert any(s.kind == "denied" for s in run.steps)
    assert "the user declined" in model.results[-1]


def test_checker_reports_broken_destinations() -> None:
    t = tools()
    assert (
        json.loads(t["check_destination"].run(t["check_destination"].args(slug="go")))["status"]
        == "ok"
    )
    status = json.loads(t["check_destination"].run(t["check_destination"].args(slug="dead")))[
        "status"
    ]
    assert status.startswith("broken:")


def test_step_and_token_budgets_stop_a_runaway_agent() -> None:
    forever = Script(*[call("list_links") for _ in range(50)])
    run = run_agent("tidy", forever, tools(), system="", confirm=lambda n, a: True, max_steps=4)  # type: ignore[arg-type]
    assert run.steps[-1].kind == "stopped" and "step budget" in run.steps[-1].detail
    greedy = Script(*[call("list_links") for _ in range(50)])
    run = run_agent("tidy", greedy, tools(), system="", confirm=lambda n, a: True, max_tokens=250)  # type: ignore[arg-type]
    assert "token budget" in run.steps[-1].detail
