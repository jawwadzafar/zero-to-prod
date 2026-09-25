"""An agent that tidies your links: finds destinations that are broken and,
with your confirmation, deletes those links (chapter 14.9).

An agent is a model in a loop, choosing its own next step with tools, until
the goal is met. That's powerful and risky, so this one has hard limits:
- a budget: at most `max_steps` model turns and `max_tokens` tokens;
- read tools run freely; the one *write* tool (delete) runs only after a
  human says yes, and snip's own permissions still apply;
- every step is recorded, so you can see exactly what it did and why.

    SNIP_URL=... SNIP_API_KEY=... uv run python -m snipai.agent
"""

from __future__ import annotations

import json
import os
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from snipai.ask import GetLinkArgs, ListLinksArgs, Tool, snip_tools
from snipai.client import SnipClient, SnipError
from snipai.llm import LLM, Message, ToolSpec, get_llm
from snipai.pages import FetchError, fetch_page

Confirm = Callable[[str, dict[str, Any]], bool]  # (tool name, arguments) -> allowed?


@dataclass
class AgentTool(Tool):
    dangerous: bool = False  # changes something: needs a human's yes first


@dataclass
class Step:
    kind: str  # "tool", "denied", "error", "answer", "stopped"
    detail: str


@dataclass
class Run:
    goal: str
    steps: list[Step] = field(default_factory=list)
    answer: str = ""
    tokens: int = 0

    def log(self, kind: str, detail: str) -> None:
        self.steps.append(Step(kind, detail))


def run_agent(
    goal: str,
    llm: LLM,
    tools: dict[str, AgentTool],
    *,
    system: str,
    confirm: Confirm,
    max_steps: int = 12,
    max_tokens: int = 20_000,
) -> Run:
    """The agent loop: like chapter 14.6's tool loop, plus budgets and
    human confirmation for dangerous tools."""
    run = Run(goal)
    messages = [Message("user", goal)]
    specs: list[ToolSpec] = [t.spec for t in tools.values()]
    for _ in range(max_steps):
        if run.tokens > max_tokens:
            run.log("stopped", f"token budget spent ({run.tokens} > {max_tokens})")
            break
        turn = llm.chat(messages, specs, system=system)
        run.tokens += turn.completion.input_tokens + turn.completion.output_tokens
        messages.append(Message("assistant", turn.text, tool_calls=turn.tool_calls))
        if not turn.tool_calls:
            run.answer = turn.text.strip() or "(the agent gave no final answer)"
            run.log("answer", run.answer)
            return run
        for call in turn.tool_calls:
            result = _execute(tools, call.name, call.arguments, confirm, run)
            messages.append(Message("tool", result, call_id=call.id, name=call.name))
    else:
        run.log("stopped", f"step budget spent ({max_steps} turns)")
    run.answer = run.answer or "(stopped before finishing)"
    return run


def _execute(
    tools: dict[str, AgentTool], name: str, arguments: dict[str, Any], confirm: Confirm, run: Run
) -> str:
    tool = tools.get(name)
    if tool is None:
        run.log("error", f"unknown tool {name!r}")
        return f"error: there is no tool called {name!r}; available: {', '.join(tools)}"
    try:
        args = tool.args.model_validate(arguments)
    except ValidationError as e:
        run.log("error", f"{name}: invalid arguments {arguments}")
        return f"error: invalid arguments for {name}: {e.errors(include_url=False)}"
    if tool.dangerous and not confirm(name, args.model_dump()):
        run.log("denied", f"{name}({json.dumps(args.model_dump())}): the human said no")
        return "not done: the user declined. Do not retry this action."
    try:
        result = tool.run(args)
    except SnipError as e:
        result = f"error: {e}"
    run.log("tool", f"{name}({json.dumps(args.model_dump())}) -> {result[:150]}")
    return result


class DeleteArgs(BaseModel):
    model_config = ConfigDict(extra="forbid")
    slug: str = Field(description="The slug of the short link to delete.")
    reason: str = Field(description="Why this link should be deleted, shown to the user.")


def tidy_tools(
    snip: SnipClient, *, fetch: Callable[[str], object] = fetch_page
) -> dict[str, AgentTool]:
    """Read tools from chapter 14.6, a link checker, and one dangerous tool.
    `fetch` loads a page or raises FetchError (tests pass a fake one)."""
    base = snip_tools(snip)

    def check_destination(a: GetLinkArgs) -> str:
        link = snip.get(a.slug)
        try:
            fetch(link.url)
            return json.dumps({"slug": a.slug, "url": link.url, "status": "ok"})
        except FetchError as e:
            return json.dumps({"slug": a.slug, "url": link.url, "status": f"broken: {e}"})

    def delete_link(a: DeleteArgs) -> str:
        snip.delete(a.slug)
        return json.dumps({"deleted": a.slug})

    def agent_tool(
        name: str,
        description: str,
        args: type[BaseModel],
        fn: Callable[[Any], str],
        dangerous: bool = False,
    ) -> AgentTool:
        return AgentTool(ToolSpec(name, description, args.model_json_schema()), args, fn, dangerous)

    return {
        "list_links": agent_tool(
            "list_links", base["list_links"].spec.description, ListLinksArgs, base["list_links"].run
        ),
        "check_destination": agent_tool(
            "check_destination",
            "Check whether a short link's destination page still loads. "
            "Returns status 'ok' or 'broken: <reason>'.",
            GetLinkArgs,
            check_destination,
        ),
        "delete_link": agent_tool(
            "delete_link",
            "Delete a short link. Only for links whose destination is broken. "
            "The user must approve each deletion.",
            DeleteArgs,
            delete_link,
            dangerous=True,
        ),
    }


SYSTEM = (
    "You tidy the user's short links in the snip link shortener. "
    "Work step by step with the tools: list the links, check each destination, "
    "and delete only links whose destination is broken, giving the reason. "
    "Never delete a link whose destination is ok. When done, summarise what you did."
)

GOAL = "Find my short links whose destination page is broken, and delete them."


def ask_human(name: str, args: dict[str, Any]) -> bool:
    reply = input(f"\nThe agent wants to run {name}({json.dumps(args)}). Allow? [y/N] ")
    return reply.strip().lower() in ("y", "yes")


def main(argv: list[str] | None = None) -> int:
    url, key = os.environ.get("SNIP_URL"), os.environ.get("SNIP_API_KEY")
    if not (url and key):
        print("set SNIP_URL and SNIP_API_KEY (chapter 14.1)", file=sys.stderr)
        return 2
    auto = os.environ.get("SNIPAI_AGENT_APPROVE")  # "all" or "none", for demos and CI
    confirm: Confirm = (lambda n, a: auto == "all") if auto else ask_human
    llm = get_llm()
    print(f"provider: {llm.name}, model: {llm.model}\ngoal: {GOAL}\n")
    with SnipClient(url, key) as snip:
        run = run_agent(GOAL, llm, tidy_tools(snip), system=SYSTEM, confirm=confirm)
    for i, step in enumerate(run.steps, start=1):
        print(f"{i:>2}. [{step.kind}] {step.detail}")
    print(f"\n{run.tokens} tokens used")
    return 0


if __name__ == "__main__":
    sys.exit(main())
