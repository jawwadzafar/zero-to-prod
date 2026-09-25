"""One small interface for calling language models, three ways (chapter 14.4).

    fake       Offline and free. Deterministic: it "answers" by returning the
               first sentence of the text you gave it. For tests and first steps.
    ollama     A free open model running on your own machine (chapter 14.11).
    anthropic  Claude, through Anthropic's official Python SDK, with your own
               API key (costs a little money per call).

Pick one with environment variables:

    SNIPAI_PROVIDER=fake|ollama|anthropic   (default: fake)
    SNIPAI_MODEL=...                        (default depends on the provider)
    SNIPAI_TEMPERATURE=0                    (ollama only; for repeatable comparisons)
    ANTHROPIC_API_KEY=...                   (for anthropic; never commit it)
    OLLAMA_URL=http://localhost:11434       (for ollama)
"""

from __future__ import annotations

import json
import os
import re
import time
from collections.abc import Iterator
from dataclasses import dataclass
from typing import Protocol

import anthropic
import httpx


@dataclass
class Completion:
    """What came back, and what it cost."""

    text: str
    model: str
    input_tokens: int
    output_tokens: int
    stop_reason: str  # "end_turn", "max_tokens", "refusal", ...
    seconds: float

    @property
    def truncated(self) -> bool:
        """The model ran out of max_tokens before finishing its answer."""
        return self.stop_reason == "max_tokens"

    def cost_usd(self) -> float:
        return estimate_cost(self.model, self.input_tokens, self.output_tokens)


class LLM(Protocol):
    """Anything that can complete a prompt. Every provider below fits it."""

    name: str
    model: str

    def complete(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Completion:
        """Send one prompt and wait for the whole answer."""
        ...

    def stream(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Iterator[str]:
        """Yield the answer piece by piece, as the model generates it."""
        ...


# US dollars per million tokens (input, output), as published at the time of
# writing. Prices change: check the provider's pricing page before relying on them.
PRICES_PER_MTOK: dict[str, tuple[float, float]] = {
    "claude-haiku-4-5": (1.00, 5.00),
    "claude-sonnet-5": (2.00, 10.00),
    "claude-opus-5": (5.00, 25.00),
}


def estimate_cost(model: str, input_tokens: int, output_tokens: int) -> float:
    """0 for local and fake models (you pay in electricity and hardware instead)."""
    price_in, price_out = PRICES_PER_MTOK.get(model, (0.0, 0.0))
    return (input_tokens * price_in + output_tokens * price_out) / 1_000_000


# ---------------------------------------------------------------------------
# fake: deterministic, offline
# ---------------------------------------------------------------------------


class FakeLLM:
    """Returns the first sentence of the page text in the prompt (inside
    <text>...</text>, or after the last 'TEXT:' marker, or of the whole
    prompt). Not intelligent, but predictable, free,
    and good enough to exercise all the code around a model call."""

    name = "fake"

    def __init__(self, model: str = "fake-extractive") -> None:
        self.model = model

    def complete(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Completion:
        start = time.perf_counter()
        if "<text>" in prompt:  # the page text, fenced in tags (chapter 14.5)
            source = prompt.rsplit("<text>", 1)[-1].split("</text>", 1)[0]
        else:
            source = prompt.rsplit("TEXT:", 1)[-1]
        match = re.search(r"[^.!?\n]*[A-Za-z][^.!?\n]*[.!?]", source)
        words = (match.group(0) if match else source).split()
        text = " ".join(words[:max_tokens])
        return Completion(
            text=text,
            model=self.model,
            input_tokens=len(((system or "") + prompt).split()),  # a rough stand-in for tokens
            output_tokens=len(text.split()),
            stop_reason="max_tokens" if len(words) > max_tokens else "end_turn",
            seconds=time.perf_counter() - start,
        )

    def stream(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Iterator[str]:
        for i, word in enumerate(
            self.complete(prompt, system=system, max_tokens=max_tokens).text.split()
        ):
            yield word if i == 0 else " " + word


# ---------------------------------------------------------------------------
# anthropic: Claude through the official SDK
# ---------------------------------------------------------------------------


class AnthropicLLM:
    """Claude via the `anthropic` package. The SDK reads ANTHROPIC_API_KEY from
    the environment, retries rate limits and server errors with backoff, and
    raises typed exceptions (anthropic.RateLimitError, ...)."""

    name = "anthropic"

    def __init__(
        self, model: str = "claude-haiku-4-5", *, client: anthropic.Anthropic | None = None
    ) -> None:
        self.model = model
        self._client = client or anthropic.Anthropic(
            timeout=60.0,  # the SDK default is 10 minutes: far too long for a web request
            max_retries=2,  # retries 429s, 5xx and connection errors, with backoff
        )

    def complete(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Completion:
        start = time.perf_counter()
        response = self._client.messages.create(
            model=self.model,
            max_tokens=max_tokens,
            system=system or anthropic.omit,
            messages=[{"role": "user", "content": prompt}],
        )
        # The answer is a list of content blocks; we want the text ones.
        text = "".join(block.text for block in response.content if block.type == "text")
        return Completion(
            text=text,
            model=response.model,
            input_tokens=response.usage.input_tokens,
            output_tokens=response.usage.output_tokens,
            stop_reason=response.stop_reason or "",
            seconds=time.perf_counter() - start,
        )

    def stream(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Iterator[str]:
        with self._client.messages.stream(
            model=self.model,
            max_tokens=max_tokens,
            system=system or anthropic.omit,
            messages=[{"role": "user", "content": prompt}],
        ) as stream:
            yield from stream.text_stream


# ---------------------------------------------------------------------------
# ollama: an open model on your own machine
# ---------------------------------------------------------------------------


class OllamaLLM:
    """Talks to a local Ollama server over its HTTP API (POST /api/chat)."""

    name = "ollama"

    def __init__(
        self,
        model: str = "qwen2.5:0.5b",
        *,
        base_url: str = "http://localhost:11434",
        temperature: float
        | None = None,  # None = the model's default; 0 = always the likeliest token
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        self.model = model
        self.temperature = temperature
        # Local models can be slow on a laptop CPU: allow two minutes.
        self._http = httpx.Client(base_url=base_url, timeout=120.0, transport=transport)

    def _body(
        self, prompt: str, system: str | None, max_tokens: int, stream: bool
    ) -> dict[str, object]:
        messages = [{"role": "system", "content": system}] if system else []
        messages.append({"role": "user", "content": prompt})
        options: dict[str, object] = {"num_predict": max_tokens}
        if self.temperature is not None:
            options |= {"temperature": self.temperature, "seed": 42}  # repeatable runs
        return {"model": self.model, "messages": messages, "stream": stream, "options": options}

    def complete(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Completion:
        start = time.perf_counter()
        res = self._http.post("/api/chat", json=self._body(prompt, system, max_tokens, False))
        res.raise_for_status()
        data = res.json()
        return Completion(
            text=data["message"]["content"],
            model=self.model,
            input_tokens=data.get("prompt_eval_count", 0),
            output_tokens=data.get("eval_count", 0),
            stop_reason="max_tokens" if data.get("done_reason") == "length" else "end_turn",
            seconds=time.perf_counter() - start,
        )

    def stream(
        self, prompt: str, *, system: str | None = None, max_tokens: int = 500
    ) -> Iterator[str]:
        body = self._body(prompt, system, max_tokens, True)
        with self._http.stream("POST", "/api/chat", json=body) as res:
            res.raise_for_status()
            for line in res.iter_lines():  # one JSON object per line
                if line:
                    yield json.loads(line)["message"]["content"]


def get_llm() -> LLM:
    """Build the provider chosen by SNIPAI_PROVIDER (default: fake)."""
    provider = os.environ.get("SNIPAI_PROVIDER", "fake")
    model = os.environ.get("SNIPAI_MODEL")
    if provider == "fake":
        return FakeLLM()
    if provider == "anthropic":
        return AnthropicLLM(model or "claude-haiku-4-5")
    if provider == "ollama":
        url = os.environ.get("OLLAMA_URL", "http://localhost:11434")
        temp = os.environ.get("SNIPAI_TEMPERATURE")
        return OllamaLLM(
            model or "qwen2.5:0.5b", base_url=url, temperature=float(temp) if temp else None
        )
    raise ValueError(f"SNIPAI_PROVIDER must be fake, ollama or anthropic, not {provider!r}")
