"""Ask questions about your snip links in plain English (chapter 14.6).

The model can't see your data. Instead, we describe *tools* (functions) it
may ask us to call. It replies with a tool call; our code checks and runs it
and sends back the result; the model uses that to answer. The model never
touches snip directly: every call goes through code we wrote and control.

    SNIP_URL=http://localhost:8080 SNIP_API_KEY=snip_... \\
        uv run python -m snipai.ask "Which of my links has the most clicks?"
"""

from __future__ import annotations

import json
import os
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from snipai.client import SnipClient, SnipError
from snipai.llm import LLM, Message, ToolSpec, get_llm


# Each tool's arguments are a pydantic model: its JSON schema is what the model
# sees, and validating the model's arguments against it is what we trust.
class ListLinksArgs(BaseModel):
    model_config = ConfigDict(extra="forbid")
    limit: int = Field(default=50, description="How many links to return, at most 100.")


class GetLinkArgs(BaseModel):
    model_config = ConfigDict(extra="forbid")
    slug: str = Field(description="The short link's slug, e.g. 'go' for https://snip.example/go.")


@dataclass
class Tool:
    spec: ToolSpec
    args: type[BaseModel]
    run: Callable[[Any], str]


def snip_tools(snip: SnipClient) -> dict[str, Tool]:
    """Read-only tools over snip's API. (Tools that change things, like
    deleting a link, need a human's confirmation first: chapter 14.9.)"""

    def list_links(a: ListLinksArgs) -> str:
        links = snip.list(limit=min(max(a.limit, 1), 100))
        return json.dumps([{"slug": x.slug, "url": x.url, "clicks": x.clicks} for x in links])

    def get_link(a: GetLinkArgs) -> str:
        x = snip.get(a.slug)
        return json.dumps(
            {
                "slug": x.slug,
                "url": x.url,
                "clicks": x.clicks,
                "created_at": x.created_at.isoformat(),
            }
        )

    def tool(name: str, description: str, args: type[BaseModel], run: Callable[[Any], str]) -> Tool:
        return Tool(ToolSpec(name, description, args.model_json_schema()), args, run)

    tools = [
        tool(
            "list_links",
            "List the user's short links with their destination URL and click count. "
            "Use this to find, compare or count links.",
            ListLinksArgs,
            list_links,
        ),
        tool(
            "get_link",
            "Get one short link by its slug, with its destination, clicks and creation time.",
            GetLinkArgs,
            get_link,
        ),
    ]
    return {t.spec.name: t for t in tools}


SYSTEM = (
    "You answer questions about the user's short links in the snip link shortener. "
    "Use the tools to look up real data; never guess numbers or links. "
    "Answer briefly, in plain English."
)


@dataclass
class Answer:
    text: str
    steps: list[str] = field(default_factory=list)  # what happened, for logs and debugging
    input_tokens: int = 0
    output_tokens: int = 0


def ask(question: str, llm: LLM, tools: dict[str, Tool], *, max_steps: int = 5) -> Answer:
    """The tool loop: ask; run any tools the model requests; repeat until it
    answers in words, or give up after max_steps (a model can loop forever)."""
    messages = [Message("user", question)]
    answer = Answer("")
    specs = [t.spec for t in tools.values()]
    for _ in range(max_steps):
        turn = llm.chat(messages, specs, system=SYSTEM)
        answer.input_tokens += turn.completion.input_tokens
        answer.output_tokens += turn.completion.output_tokens
        messages.append(Message("assistant", turn.text, tool_calls=turn.tool_calls))
        if not turn.tool_calls:
            # Small models sometimes reply with nothing, or announce a tool call
            # ("let me look that up...") without making one. Report, don't guess.
            answer.text = turn.text.strip() or "(the model gave no answer)"
            return answer
        for call in turn.tool_calls:
            result = run_tool(tools, call.name, call.arguments)
            answer.steps.append(f"{call.name}({json.dumps(call.arguments)}) -> {result[:120]}")
            messages.append(Message("tool", result, call_id=call.id, name=call.name))
    answer.text = f"(stopped after {max_steps} steps without a final answer)"
    return answer


def run_tool(tools: dict[str, Tool], name: str, arguments: dict[str, Any]) -> str:
    """Never trust a tool call: check the tool exists and the arguments are
    valid, and turn every failure into a message the model can read and react to."""
    tool = tools.get(name)
    if tool is None:
        return f"error: there is no tool called {name!r}; available: {', '.join(tools)}"
    try:
        args = tool.args.model_validate(arguments)
    except ValidationError as e:
        return f"error: invalid arguments for {name}: {e.errors(include_url=False)}"
    try:
        return tool.run(args)
    except SnipError as e:
        return f"error: {e}"


def main(argv: list[str] | None = None) -> int:
    question = (
        " ".join(argv if argv is not None else sys.argv[1:])
        or "Which of my links has the most clicks?"
    )
    url, key = os.environ.get("SNIP_URL"), os.environ.get("SNIP_API_KEY")
    if not (url and key):
        print("set SNIP_URL and SNIP_API_KEY (chapter 14.1)", file=sys.stderr)
        return 2
    llm = get_llm()
    print(f"provider: {llm.name}, model: {llm.model}\nquestion: {question}\n")
    with SnipClient(url, key) as snip:
        answer = ask(question, llm, snip_tools(snip))
    for step in answer.steps:
        print(f"  tool: {step}")
    print(f"\nanswer: {answer.text}")
    print(f"({answer.input_tokens} tokens in, {answer.output_tokens} out)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
